// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/cache"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/iofactory"
	"github.com/dustin/go-humanize"
)

func verifyObject(o *hos.Object, fields ...string) error {
	find := func(name string) bool {
		return slices.Contains(fields, name)
	}

	if o == nil {
		return hos.ErrNotInitialized
	}
	if o.PoolID == "" && find("PoolID") {
		return fmt.Errorf("pool id is required, %w", hos.ErrBadRequest)
	}
	if o.Name == "" && find("Name") {
		return fmt.Errorf("name is required, %w", hos.ErrBadRequest)
	}
	if o.ID == "" && find("ID") {
		return fmt.Errorf("id is required, %w", hos.ErrBadRequest)
	}
	if o.ContentType == "" && find("ContentType") {
		return hos.ErrContentTypeRequired
	}
	if o.Size == 0 && find("Size") {
		return hos.ErrSizeRequired
	}
	return nil
}

// CreateObject uploads a new object to the specified pool with automatic replication
func (c *Client) CreateObject(ctx context.Context, o *hos.Object, rcs iofactory.ReadClosers, opts ...Options) (*hos.Object, error) {
	var (
		onBehalf    Options
		sizeUnknown bool
	)
	for _, opt := range opts {
		if ob, ok := opt.(OnBehalf); ok {
			onBehalf = ob
			break
		}
		if headers, ok := opt.(Headers); ok {
			sizeUnknown = headers[header.SizeUnknown] != ""
		}
	}

	fields := []string{"PoolID", "Name", "ContentType"}
	if !sizeUnknown {
		fields = append(fields, "Size")
	}
	if err := verifyObject(o, fields...); err != nil {
		return nil, err
	}

	// create missing pools
	pool, err := c.GetPool(ctx, o.PoolID, onBehalf)
	if err != nil {
		pool, err = c.createMissingPools(ctx, o.PoolID, onBehalf)
		if err != nil && !errors.Is(err, errUserNotSame) {
			return nil, err
		}
	}

	if err := checkEncryptionKey(pool, modifiers(opts...)...); err != nil {
		return nil, err
	}

	// we assign new error value here, because we use the err below
	// to check errUserNotSame error
	serverInfo, errInfo := c.GetServerInfo(ctx, IgnoreErrors(hos.ErrConnectionFailure), onBehalf)
	if errInfo != nil {
		return nil, errInfo
	}

	if pool.ReplicaCount > len(serverInfo) {
		return nil, fmt.Errorf("replica count %d cannot be bigger than available servers %d, %w",
			pool.ReplicaCount, len(serverInfo), hos.ErrInsufficientResources)
	}

	// remove servers with insufficient space
	serversWithAvailableSpace := slices.DeleteFunc(serverInfo, func(info hos.ServerInfo) bool {
		return info.FreeDisk() <= uint64(o.Size+1024)
	})

	if pool.ReplicaCount > len(serversWithAvailableSpace) {
		return nil, fmt.Errorf("number of servers that have required disk space %s are %d is less than replica count %d, %w",
			humanize.Bytes(uint64(o.Size)), len(serversWithAvailableSpace), pool.ReplicaCount, hos.ErrInsufficientResources)
	}

	// get the servers where the pool is exists on
	servers := []url.URL{}
	// user is not the owner of the Pool
	if errors.Is(err, errUserNotSame) {
		poolServerURLs, err := c.getPoolServers(ctx, o.PoolID, onBehalf, IgnoreErrors(hos.ErrNotExist, hos.ErrConnectionFailure))
		if err != nil {
			return nil, err
		}
		slices.Sort(poolServerURLs)

		for _, s := range serversWithAvailableSpace {
			if _, found := slices.BinarySearch(poolServerURLs, s.URL.String()); !found {
				continue
			}

			servers = append(servers, *s.URL)
			if len(servers) == pool.ReplicaCount {
				break
			}
		}
	} else {
		for _, s := range serversWithAvailableSpace[:pool.ReplicaCount] {
			servers = append(servers, *s.URL)
		}
	}

	if pool.ReplicaCount > len(servers) {
		return nil, fmt.Errorf("replica count %d cannot be bigger than available servers %d, %w",
			pool.ReplicaCount, len(servers), hos.ErrInsufficientResources)
	}

	modifiersList := append(modifiers(opts...), &readClosers{rcs}, Headers(header.Serialize(o)))
	responses := c.doP(context.Background(), "PUT", path.Join(constant.APIPrefix, o.PoolID), servers, modifiersList...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return getOne[hos.Object](responses)
}

// EditObject modifies an existing object's metadata
func (c *Client) EditObject(ctx context.Context, o *hos.Object, opts ...Options) (*hos.Object, error) {
	if o == nil {
		return nil, fmt.Errorf("object %w", hos.ErrNotInitialized)
	}

	// check the object
	serverURLs, _, err := c.getObjectsServers(ctx, o.PoolID, o.ID, opts...)
	if err != nil {
		return nil, err
	}

	modifiersList := append(modifiers(opts...), Headers(header.Serialize(o)))
	responses := c.doP(ctx, "POST", path.Join(constant.APIPrefix, o.PoolID, o.ID), serverURLs, modifiersList...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return getOne[hos.Object](responses)
}

func (c *Client) getObjectsServers(ctx context.Context, pid, oid string, opts ...Options) ([]url.URL, []hos.Object, error) {
	// check the object
	headResponses := c.doP(ctx, "HEAD", path.Join(constant.APIPrefix, pid, oid), c.servers, modifiers(opts...)...)
	serverURLs, objs, err := getMany[hos.Object](headResponses, errHandlers(opts...)...)
	if err != nil {
		return nil, nil, err
	}

	errorHandlers := errHandlers(opts...)
	// objs length cannot be 0 it is checked in getMany function
	if objectCount := len(objs); objectCount < objs[0].ReplicaCount {
		// check errors
		if err := handleErrors(headResponses, errorHandlers...); err != nil {
			return nil, nil, errors.Join(err, hos.ErrNotAllCopiesAvailable)
		}
		// in case we ignore ErrNotAllCopiesAvailable
		var notAllCopiesErr error = hos.ErrNotAllCopiesAvailable
		for _, eh := range errorHandlers {
			notAllCopiesErr = eh.HandleError(notAllCopiesErr)
		}
		if notAllCopiesErr != nil {
			return nil, nil, notAllCopiesErr
		}
	} else if objectCount > objs[0].ReplicaCount {
		// this should normally never happened
		var corruptedErr error = hos.ErrCorrupted
		for _, eh := range errorHandlers {
			corruptedErr = eh.HandleError(corruptedErr)
		}

		if corruptedErr != nil {
			return nil, nil, corruptedErr
		}
	}

	return serverURLs, objs, nil
}

// GetObject retrieves object metadata by pool ID and object ID
func (c *Client) GetObject(ctx context.Context, pid, oid string, opts ...Options) (*hos.Object, error) {
	if pid == "" {
		return nil, fmt.Errorf("pool id cannot be empty %w", hos.ErrBadRequest)
	}

	_, objects, err := c.getObjectsServers(ctx, pid, oid, opts...)
	if err != nil {
		return nil, err
	}

	return &objects[0], nil
}

// GetContent downloads the object's data content from the cluster
func (c *Client) GetContent(ctx context.Context, pid, oid string, opts ...Options) (*hos.Object, error) {
	if pid == "" {
		return nil, fmt.Errorf("pool id cannot be empty %w", hos.ErrBadRequest)
	}

	var (
		serverURLs []url.URL
		err        error
	)

	if !c.pinServer {
		serverURLs, _, err = c.getObjectsServers(ctx, pid, oid, opts...)
		if err != nil {
			return nil, err
		}
	} else {
		pinnedServer, ok := cache.Get[string, url.URL](c.pinMap, oid)
		if !ok {
			servers, _, err := c.getObjectsServers(ctx, pid, oid, opts...)
			if err != nil {
				return nil, err
			}
			serverURLs = []url.URL{servers[0]}
			cache.Set(c.pinMap, oid, serverURLs[0], time.Minute*30)
		} else {
			serverURLs = []url.URL{pinnedServer}
		}
	}

	responses := c.doP(ctx, "GET", path.Join(constant.APIPrefix, pid, oid), serverURLs, modifiers(opts...)...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return getOne[hos.Object](responses)
}

// ListObjects returns all objects within the specified pool
func (c *Client) ListObjects(ctx context.Context, pid string, opts ...Options) ([]hos.Object, error) {
	if pid == "" {
		return nil, fmt.Errorf("pool id cannot be empty %w", hos.ErrBadRequest)
	}
	responses := c.doP(ctx, "GET", path.Join(constant.APIPrefix, pid), c.servers, modifiers(opts...)...)

	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	objects, err := merge[hos.Object](responses, filters(opts...)...)
	if err != nil {
		return nil, err
	}

	return objects, nil
}

// MoveObject moves an object between pools or renames it within the same pool
func (c *Client) MoveObject(ctx context.Context, pid, oid, dpid, nname string, opts ...Options) error {
	if pid == "" || oid == "" || dpid == "" {
		return fmt.Errorf("id cannot be empty %w", hos.ErrBadRequest)
	}

	serverURLs, _, err := c.getObjectsServers(ctx, pid, oid, opts...)
	if err != nil {
		return err
	}

	if nname != "" {
		opts = append(opts, Headers(map[string]string{header.NewObjectName: nname}))
	}

	responses := c.doP(ctx, "PATCH", path.Join(constant.APIPrefix, pid, oid, dpid), serverURLs, modifiers(opts...)...)
	return handleErrors(responses, errHandlers(opts...)...)
}

// DeleteObject removes an object from all servers in the cluster
func (c *Client) DeleteObject(ctx context.Context, pid, oid string, opts ...Options) error {
	if pid == "" {
		return fmt.Errorf("pool id cannot be empty %w", hos.ErrBadRequest)
	}

	serverURLs, _, err := c.getObjectsServers(ctx, pid, oid, opts...)
	if err != nil {
		return err
	}

	responses := c.doP(ctx, "DELETE", path.Join(constant.APIPrefix, pid, oid), serverURLs, modifiers(opts...)...)
	errorHandlerOptions := append(errHandlers(opts...), IgnoreErrors(hos.ErrNotExist))
	return handleErrors(responses, errorHandlerOptions...)
}
