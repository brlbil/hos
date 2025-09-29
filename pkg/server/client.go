// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/fs"
	"github.com/brlbil/hos/internal/header"
	"github.com/minio/sio"
	"golang.org/x/net/http2"
)

type clientFunc func(*http.Request) error

const userAgent = "hos-server"

type client struct {
	clt *http.Client
	srv *url.URL
}

// TODO: timeout is not configured properly
func newClient(addr, cert string) (*client, error) {
	c := &http.Client{}
	if cert != "" {
		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM([]byte(cert))
		c.Transport = &http2.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caPool,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
			},
			DisableCompression: true,
			AllowHTTP:          false,
		}
	} else {
		c.Timeout = time.Second
		c.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint: gosec
		}
	}
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %s", u.Scheme)
	}

	return &client{clt: c, srv: u}, nil
}

func headers(hh map[string]string) clientFunc {
	return func(r *http.Request) error {
		for k, v := range hh {
			r.Header.Set(k, v)
		}
		return nil
	}
}

func setHeader(key, value string) clientFunc {
	return func(r *http.Request) error {
		r.Header.Set(key, value)
		return nil
	}
}

func uploadEncObject(fs *fs.FS, o *hos.Object, encf *sio.Config) clientFunc {
	return func(r *http.Request) error {
		rsc, err := fs.ReadObject(context.Background(), o, encf)
		if err != nil {
			return err
		}
		if o.Encrypted && encf == nil {
			l, err := rsc.Seek(0, io.SeekEnd)
			if err != nil {
				return err
			}
			r.ContentLength = l
			_, err = rsc.Seek(0, io.SeekStart)
			if err != nil {
				return err
			}
		}
		r.Body = rsc
		r.GetBody = func() (io.ReadCloser, error) {
			return fs.ReadObject(context.Background(), o, encf)
		}
		return nil
	}
}

func uploadBuf(b []byte) clientFunc {
	return func(r *http.Request) error {
		r.ContentLength = int64(len(b))
		r.Body = io.NopCloser(bytes.NewReader(b))
		r.GetBody = func() (io.ReadCloser, error) {
			r.ContentLength = int64(len(b))
			return io.NopCloser(bytes.NewBuffer(b)), nil
		}
		return nil
	}
}

func uploadFile(name string) clientFunc {
	return func(r *http.Request) error {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		r.Body = file
		r.GetBody = func() (io.ReadCloser, error) {
			return os.Open(name)
		}
		return nil
	}
}

func marshalJSON[T any](t T) clientFunc {
	return func(r *http.Request) error {
		if v := any(t); v == nil {
			return nil
		}
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		r.ContentLength = int64(len(data))
		r.Header.Set("Content-Type", "application/json")
		r.Body = io.NopCloser(bytes.NewReader(data))
		return nil
	}
}

func marshalHeader[T any](t T) clientFunc {
	hh := header.Serialize(t)
	return headers(hh)
}

func setToken(token string) clientFunc {
	return func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

func setQuery(ss string) clientFunc {
	return func(r *http.Request) error {
		val := r.URL.Query()
		for _, s := range strings.Split(ss, ",") {
			kv := strings.Split(s, "=")
			if len(kv) != 2 && (kv[0] == "" || kv[1] == "") {
				continue
			}
			val.Set(kv[0], kv[1])
		}
		r.URL.RawQuery = val.Encode()
		return nil
	}
}

func (c *client) do(ctx context.Context, method, path string, cFns ...clientFunc) (*http.Response, error) {
	srv := *c.srv
	srv.Path = path
	req, err := http.NewRequestWithContext(ctx, method, srv.String(), nil)
	if err != nil {
		return nil, err
	}

	for _, fn := range cFns {
		if err := fn(req); err != nil {
			return nil, err
		}
	}
	// set user agent
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.clt.Do(req)
	if err != nil {
		return resp, err
	}

	return resp, nil
}
