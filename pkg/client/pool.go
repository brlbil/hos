// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/pkg/id"
)

func (c *Client) createMissingPools(ctx context.Context, pid string, onBehalf Options) (*hos.Pool, error) {
	// let's first check if we are dealing with a linked pool
	pr, err := c.GetPool(ctx, pid, NoRedirect(), IgnoreErrors(hos.ErrNotExist, hos.ErrConnectionFailure), onBehalf)
	if err != nil {
		return nil, err
	}

	// even if Pool is linked to another, check if target Pool exists on all the servers
	p, err := c.GetPool(ctx, pid, IgnoreErrors(hos.ErrNotExist, hos.ErrConnectionFailure), onBehalf)
	if err != nil {
		return nil, err
	}

	if pr.UserID != p.UserID {
		return p, errUserNotSame
	}

	// this will create the Pool in every server
	// it ignores if the Pool exist on all the Servers
	p1, err := c.createPool(ctx, p, IgnoreErrors(hos.ErrConnectionFailure), onBehalf)
	if err != nil && !errors.Is(err, hos.ErrExist) {
		return nil, err
	} else if err == nil {
		p = p1
	}

	// the original Pool is linked to another and let's create it
	// on all the servers if we are missing any
	if pr.LinkedID != "" {
		_, err := c.createPool(ctx, pr, onBehalf)
		if err != nil && !errors.Is(err, hos.ErrExist) {
			return nil, err
		}
	}

	return p, nil
}

func (c *Client) createPool(ctx context.Context, p *hos.Pool, opts ...Options) (*hos.Pool, error) {
	responses := c.doP(ctx, "PUT", constant.APIPrefix, c.servers, append(modifiers(opts...), Headers(header.Serialize(p)))...)
	err := handleErrors(responses, append(errHandlers(opts...), IgnoreErrors(hos.ErrExist))...)
	if err != nil {
		return nil, err
	}

	return getOne[hos.Pool](responses)
}

// CreatePool creates a new pool across all configured servers with the specified replication settings
func (c *Client) CreatePool(ctx context.Context, p *hos.Pool, opts ...Options) (*hos.Pool, error) {
	userID := id.Gen(c.user)
	for _, opt := range opts {
		if onBehalfUser, ok := opt.(OnBehalf); ok {
			userID = id.Gen(string(onBehalfUser))
			break
		}
	}

	// get existing pools
	poolID := p.ID
	if poolID == "" {
		poolID = id.Gen(userID, p.Name)
	}
	pool, err := c.GetPool(ctx, poolID, append(opts, IgnoreErrors(hos.ErrNotExist))...)
	if err != nil && !errors.Is(err, hos.ErrNotExist) {
		return nil, err
	}

	// pool is exists, at least on some of the servers
	// fmt.Println("-", pool.ID)
	if pool != nil {
		if p.ReplicaCount != 0 && p.ReplicaCount != pool.ReplicaCount {
			return nil, fmt.Errorf(
				"pool %s replica count is %d and create request replica count is %d, %w",
				p.Name, pool.ReplicaCount, p.ReplicaCount, hos.ErrBadRequest)
		}
		if p.Encrypted && !pool.Encrypted {
			return nil, fmt.Errorf("existing pool(s) %s is not encrypted and create request is encrypted, %w", p.Name, hos.ErrBadRequest)
		}
		if len(p.Attributes) != 0 && !maps.Equal(pool.Attributes, p.Attributes) {
			return nil, fmt.Errorf("pool %s has attributes and create request attributes does not match them, %w", p.Name, hos.ErrBadRequest)
		}
		if len(p.Labels) != 0 && !maps.Equal(pool.Labels, p.Labels) {
			return nil, fmt.Errorf("pool %s has labels and create request labels does not match them, %w", p.Name, hos.ErrBadRequest)
		}
		p.Encrypted = pool.Encrypted
		p.ReplicaCount = pool.ReplicaCount
		if pool.Attributes != nil {
			p.Attributes = map[string]string{}
			maps.Copy(p.Attributes, pool.Attributes)
		}
		if pool.Labels != nil {
			p.Labels = map[string]string{}
			maps.Copy(p.Labels, pool.Labels)
		}
	}

	return c.createPool(ctx, p, opts...)
}

// EditPool modifies an existing pool's metadata
func (c *Client) EditPool(ctx context.Context, p *hos.Pool, opts ...Options) (*hos.Pool, error) {
	if p == nil {
		return nil, fmt.Errorf("pool %w", hos.ErrNotInitialized)
	}

	// check the object
	objectResponses := c.doP(ctx, "HEAD", path.Join(constant.APIPrefix, p.ID), c.servers, modifiers(opts...)...)
	serverURLs, _, err := getMany[hos.Pool](objectResponses, errHandlers(opts...)...)
	if err != nil {
		return nil, err
	}

	reqPath := path.Join(constant.APIPrefix, p.ID)
	responses := c.doP(ctx, "POST", reqPath, serverURLs, append(modifiers(opts...), Headers(header.Serialize(p)))...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return getOne[hos.Pool](responses)
}

// GetPool retrieves pool information by ID from the cluster
func (c *Client) GetPool(ctx context.Context, id string, opts ...Options) (*hos.Pool, error) {
	responses := c.doP(ctx, "HEAD", path.Join(constant.APIPrefix, id), c.servers, modifiers(opts...)...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return getOne[hos.Pool](responses)
}

// getPoolServers gets the servers that Pools are exist on
func (c *Client) getPoolServers(ctx context.Context, id string, onBehalf Options, errorHandlers ...ErrorHandler) ([]string, error) {
	responses := c.doP(ctx, "HEAD", path.Join(constant.APIPrefix, id), c.servers, modifiers(onBehalf)...)

	if err := handleErrors(responses, errorHandlers...); err != nil {
		return nil, err
	}

	servers := []string{}
	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode != 204) {
			continue
		}

		servers = append(servers, r.url.String())
	}
	return servers, nil
}

// ListPools returns all pools accessible by the authenticated user
func (c *Client) ListPools(ctx context.Context, opts ...Options) ([]hos.Pool, error) {
	responses := c.doP(ctx, "GET", constant.APIPrefix, c.servers, modifiers(opts...)...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	pools, err := merge[hos.Pool](responses, filters(opts...)...)
	if err != nil {
		return nil, err
	}

	return pools, nil
}

// DeletePool removes a pool from all servers. The pool must be empty (no objects)
func (c *Client) DeletePool(ctx context.Context, id string, opts ...Options) error {
	// check if all the servers are available
	if _, err := c.Health(ctx, errHandlers(opts...)...); err != nil {
		return err
	}

	getOpts := append(opts, IgnoreErrors(hos.ErrNotExist), NoRedirect())
	pool, err := c.GetPool(ctx, id, getOpts...)
	if err != nil {
		return err
	}
	if pool.ObjectCount != 0 {
		return fmt.Errorf("object count %d, %w", pool.ObjectCount, hos.ErrNotEmpty)
	}

	responses := c.doP(ctx, "DELETE", path.Join(constant.APIPrefix, id), c.servers, append(modifiers(opts...), NoRedirect())...)
	return handleErrors(responses, append(errHandlers(opts...), IgnoreErrors(hos.ErrNotExist))...)
}
