// SPDX-License-Identifier: MIT

// Package client provides a Go client library for the HOS (Home Object Storage) system.
// It handles communication with HOS servers for managing pools, objects, and users.
package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/utils"
	"github.com/brlbil/hos/pkg/crypto"
	"golang.org/x/net/http2"
)

const userAgent = "hos-client"

// ConfigFunc modifies client behavior during initialization
type ConfigFunc func(*Client) error

// SetTimeout sets the default timeout for client requests
func SetTimeout(t time.Duration) ConfigFunc {
	return func(c *Client) error {
		c.defaultTimeout = t
		return nil
	}
}

// SetUserKey configures client authentication using a user's private key
func SetUserKey(userName, prvKey string) ConfigFunc {
	return func(c *Client) error {
		var tok string
		pk, err := crypto.ParsePrivateKey(prvKey)
		if err != nil {
			return fmt.Errorf("failed to parse user %s private key: %w", userName, err)
		}
		tok = pk.SignUser(userName)

		pb, err := pk.PublicKey()
		if err != nil {
			return fmt.Errorf("failed to generate public key: %w", err)
		}
		c.user = userName
		c.pubKey = pb

		if c.authFn != nil {
			return fmt.Errorf("client authentication is already set")
		}
		c.authFn = Headers(map[string]string{"Authorization": "Bearer " + tok})
		return nil
	}
}

// DebugLogging enables debug output for client requests
func DebugLogging(c *Client) error {
	c.debug = true
	return nil
}

func notUseHTTP2(c *Client) error {
	c.http1 = true
	return nil
}

// PinContentServer enables server pinning for content requests
func PinContentServer(c *Client) error {
	c.pinServer = true
	return nil
}

// Client represents a HOS client that communicates with one or more HOS servers.
// It handles parallel requests across the configured servers in a cluster.
type Client struct {
	servers        []url.URL
	clients        map[string]*http.Client
	pinMap         *sync.Map
	authFn         RequestModifier
	user           string
	pubKey         crypto.PublicKey
	defaultTimeout time.Duration
	pinServer      bool
	http1          bool
	debug          bool
}

// ServerConfig represents the configuration for a single HOS server
type ServerConfig struct {
	Address     string `json:"address,omitempty"`
	Cordoned    bool   `json:"cordoned,omitempty"`
	Certificate string `json:"certificate,omitempty"`
}

// New creates a new HOS client configured to communicate with the specified servers
func New(servers []ServerConfig, opts ...ConfigFunc) (*Client, error) {
	if len(servers) == 0 {
		return nil, errors.New("at least  one server is required")
	}

	caPool := x509.NewCertPool()

	ss := []url.URL{}
	for _, srv := range servers {
		s := srv.Address
		if !strings.Contains(s, "://") {
			s = "https://" + s
		}
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}

		if u.Scheme != "https" {
			return nil, fmt.Errorf("unsupported scheme %s", u.Scheme)
		}

		caPool.AppendCertsFromPEM([]byte(srv.Certificate))

		ss = append(ss, *u)
	}

	c := &Client{
		clients:        map[string]*http.Client{},
		servers:        ss,
		defaultTimeout: time.Second * 10,
		pinMap:         &sync.Map{},
	}

	for _, ofn := range opts {
		if err := ofn(c); err != nil {
			return nil, err
		}
	}

	var transport http.RoundTripper

	// since http1 is not public configuration func we can only configure it here
	if c.http1 {
		// http1 is configured for testing, for some reason using http2 during testing takes too much time
		transport = &http.Transport{
			// Original configurations from `http.DefaultTransport` variable.
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			// ForceAttemptHTTP2:     true, // Set it to false to enforce HTTP/1
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,

			// Our custom configurations.
			ResponseHeaderTimeout: 10 * time.Second,
			DisableCompression:    true,
			// Set DisableKeepAlives to true when using HTTP/1 otherwise it will cause error: dial tcp [::1]:8090: socket: too many open files
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				RootCAs:            caPool,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
			},
		}
	} else {
		transport = &http2.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caPool,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
			},
			DisableCompression: true,
			AllowHTTP:          false,
		}
	}

	for _, s := range c.servers {
		c.clients[s.String()] = &http.Client{
			Transport: transport,
		}
	}

	return c, nil
}

// Reconfigure applies new configuration options to an existing client
func (c *Client) Reconfigure(fns ...ConfigFunc) error {
	for _, fn := range fns {
		if err := fn(c); err != nil {
			return err
		}
	}
	return nil
}

// User returns the authenticated username for this client
func (c *Client) User() string {
	return c.user
}

func (c *Client) logDebug(v ...any) {
	if c.debug {
		log.Println(v...)
	}
}

type response struct {
	rsp *http.Response
	err error
	url url.URL
}

func (c *Client) doP(ctx context.Context, method, path string, servers []url.URL, opts ...RequestModifier) []response {
	var wg sync.WaitGroup
	serverCount := len(servers)
	responseChannel := make(chan response, serverCount)

	for _, s := range servers {
		wg.Add(1)

		go func(server url.URL, w *sync.WaitGroup) {
			resp, err := c.do(ctx, method, server, path, opts...)
			responseChannel <- response{url: server, rsp: resp, err: err}
			w.Done()
		}(s, &wg)
	}

	var responses []response
	var count int
	for r := range responseChannel {
		count++
		responses = append(responses, r)
		if count == serverCount {
			close(responseChannel)
		}
	}

	// wait for all the go routines to finish
	wg.Wait()

	return responses
}

func (c *Client) do(ctx context.Context, method string, server url.URL, path string, opts ...RequestModifier) (*http.Response, error) {
	httpClient, ok := c.clients[server.String()]
	if !ok {
		return nil, fmt.Errorf("no such server %s is configured", server.Host)
	}

	server.Path = path
	req, err := http.NewRequestWithContext(ctx, method, server.String(), nil)
	if err != nil {
		return nil, err
	}

	// set user agent
	req.Header.Set("User-Agent", userAgent)
	if c.authFn != nil {
		opts = append(opts, c.authFn)
	}
	for _, modifier := range opts {
		if err := modifier.ModifyRequest(req); err != nil {
			return nil, err
		}
	}

	c.logDebug("making", method, "request to", server.String())
	c.logDebug("request headers", req.Header)

	resp, err := httpClient.Do(req)
	if err != nil {
		return resp, err
	}

	c.logDebug("got response", resp.Status, "from", server.String())
	c.logDebug("response headers", resp.Header)

	return resp, nil
}

func getOneBody(responses []response) ([]byte, error) {
	var (
		buf []byte
		err error
	)

	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && (r.rsp.StatusCode >= 400 || r.rsp.ContentLength == 0)) {
			continue
		}

		buf, err = io.ReadAll(io.LimitReader(r.rsp.Body, r.rsp.ContentLength))
		break
	}
	return buf, err
}

func getOne[T hos.Pool | hos.Object](responses []response) (*T, error) {
	var previousURL url.URL

	results := []*T{}
	for i, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}

		t, err := fromResponse[T](r.rsp)
		if err != nil {
			return nil, fmt.Errorf("%s %w", r.url.String(), err)
		}

		if len(results) > i {
			if err := utils.Diff(results[i-1], t); err != nil {
				return nil, errors.Join(fmt.Errorf("results from %s, and %s are not equal", r.url.String(), previousURL.String()), err, hos.ErrNotEqual)
			}
		}

		results = append(results, t)
		previousURL = r.url
	}

	if len(results) == 0 {
		return nil, handleErrors(responses)
	}

	pools, ok := any(results).([]*hos.Pool)
	// this is object
	if !ok {
		return results[0], nil
	}

	p := pools[0]
	if p.ReplicaCount == 0 {
		return results[0], nil
	}

	var (
		objectCount int
		totalSize   int64
		crtTime     time.Time
		modTime     time.Time
		hash        string
	)

	for i, pool := range pools {
		if i == 0 {
			hash = pool.Hash
			crtTime = pool.CreatedAt
			modTime = pool.ModifiedAt
		} else {
			hash = calHash(hash, pool.Hash)
			if pool.CreatedAt.Before(crtTime) {
				crtTime = pool.CreatedAt
			}
			if pool.ModifiedAt.After(modTime) {
				modTime = pool.ModifiedAt
			}
		}
		objectCount += pool.ObjectCount
		totalSize += pool.Size
	}

	p.ObjectCount = objectCount / p.ReplicaCount
	p.Size = totalSize / int64(p.ReplicaCount)
	p.Hash = hash
	p.CreatedAt = crtTime
	p.ModifiedAt = modTime

	return any(p).(*T), nil
}

func merge[T hos.Pool | hos.Object | hos.User](responses []response, filters ...responseFilters) ([]T, error) {
	results := []T{}

	itemIndexMap := map[string]int{}
	reqHost := map[string]string{}

	var multiErr error

	count := 0
	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}

		opt, err := header.Parse[filter.Headers](r.rsp.Header)
		if err == nil && len(opt.Range) == 2 && opt.Range[1] > count {
			count = opt.Range[1]
		}

		lr := io.LimitReader(r.rsp.Body, r.rsp.ContentLength)
		b, err := io.ReadAll(lr)
		if err != nil {
			multiErr = errors.Join(multiErr, fmt.Errorf("%s %w", r.url.String(), err))
			continue
		}

		var tmpList []T
		if err := json.Unmarshal(b, &tmpList); err != nil {
			multiErr = errors.Join(multiErr, fmt.Errorf("%s %w", r.url.String(), err))
			continue
		}

		for _, t := range tmpList {
			id := ""
			switch v := any(t).(type) {
			case hos.Pool:
				id = v.ID
			case hos.Object:
				id = v.ID
			case hos.User:
				id = v.ID
			}

			if inx, ok := itemIndexMap[id]; ok {
				existingItem := results[inx]
				if err := utils.Diff(&existingItem, &t); err != nil {
					multiErr = errors.Join(multiErr, fmt.Errorf("results from %s, and %s are not equal", r.url.Host, reqHost[id]), err, hos.ErrNotEqual)
				}

				combinePool(&existingItem, &t)
				results[inx] = existingItem

				continue
			}

			results = append(results, t)
			itemIndexMap[id] = len(results) - 1
			reqHost[id] = r.url.Host
		}
	}

	filters = append(filters, &poolCorrector{})
	filters = append(filters, &serverAddrFilter{hostMap: reqHost})
	// run filter functions
	for _, fs := range filters {
		filteredResults := fs.Filter(results)
		results = filteredResults.([]T)
	}

	if count > 0 {
		if count > len(results) {
			count = len(results)
		}

		results = results[:count]
	}

	if multiErr != nil {
		return nil, multiErr
	}

	return results, nil
}

// combinePool increases the object count and size of the Pool
// only support pool but in order to use with in generic func merge
// this func also support all the types merge supports
func combinePool[T hos.Pool | hos.Object | hos.User](t1, t2 *T) {
	switch v1 := any(t1).(type) {
	case *hos.Pool:
		v2 := any(t2).(*hos.Pool)
		v1.ObjectCount += v2.ObjectCount
		v1.Size += v2.Size
		if v2.CreatedAt.Before(v1.CreatedAt) {
			v1.CreatedAt = v2.CreatedAt
		}
		if v2.ModifiedAt.After(v1.ModifiedAt) {
			v1.ModifiedAt = v2.ModifiedAt
		}
		v1.Hash = calHash(v1.Hash, v1.Hash)
	}
}

func convert(responses []response) ([]hos.ServerInfo, error) {
	results := []hos.ServerInfo{}

	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}

		t, err := fromResponse[hos.ServerInfo](r.rsp)
		if err != nil {
			return nil, fmt.Errorf("%s %w", r.url.String(), err)
		}

		results = append(results, *t)
	}

	slices.SortFunc(results, func(a, b hos.ServerInfo) int {
		if a.Operations != b.Operations {
			if a.Operations < b.Operations {
				return -1
			}
			return +1
		}
		af := a.FreeDisk()
		bf := b.FreeDisk()
		if af < bf {
			return -1
		}
		if af > bf {
			return +1
		}
		return 0
	})

	return results, nil
}

func getMany[T hos.Object | hos.Pool](responses []response, errorHandlers ...ErrorHandler) ([]url.URL, []T, error) {
	results := []T{}
	servers := []url.URL{}
	previousURL := ""

	var multiErr error

	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}

		t, err := fromResponse[T](r.rsp)
		if err != nil {
			return nil, nil, fmt.Errorf("%s %w", r.url.String(), err)
		}

		i := len(results)
		if i > 0 {
			if err := utils.Diff(&results[i-1], t); err != nil {
				multiErr = errors.Join(multiErr, fmt.Errorf("results from %s, and %s are not equal", r.url.String(), previousURL), err, hos.ErrNotEqual)
			}
		}
		results = append(results, *t)
		servers = append(servers, r.url)
		previousURL = r.url.String()
	}

	if len(results) == 0 {
		return nil, nil, hos.ErrNotExist
	}

	for _, eh := range errorHandlers {
		multiErr = eh.HandleError(multiErr)
	}
	if multiErr != nil {
		return nil, nil, multiErr
	}

	return servers, results, nil
}
