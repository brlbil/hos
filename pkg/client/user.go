// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"path"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
)

const (
	adminID = "880e0d76"
	anonID  = "00000000"
)

// GetUsage returns storage usage statistics for the user
func (c *Client) GetUsage(ctx context.Context, opts ...Options) (*hos.Usage, error) {
	var onBehalfUser string
	for _, opt := range opts {
		if on, ok := opt.(OnBehalf); ok {
			onBehalfUser = string(on)
			break
		}
	}

	user := c.user
	if onBehalfUser != "" {
		user = onBehalfUser
	}

	pools, err := c.ListPools(ctx, opts...)
	if err != nil {
		return nil, err
	}

	poolCount := 0
	usage := &hos.Usage{Name: user}

	for _, p := range pools {
		if p.LinkedID != "" {
			continue
		}
		usage.Object += p.ObjectCount
		usage.Size += p.Size
		poolCount++
	}
	usage.Pools = poolCount

	return usage, nil
}

// CreateUser registers a new user with their public key (admin only)
func (c *Client) CreateUser(ctx context.Context, user *hos.User, opts ...Options) error {
	if c.user != constant.AdminUser {
		return hos.ErrAdminOnly
	}
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return err
	}

	b, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("encoding json failed: %w", err)
	}

	responses := c.doP(ctx, "PUT", constant.UserAPIPrefix, c.servers, append(modifiers(opts...), jsonBody(b))...)
	return handleErrors(responses, append(errHandlers(opts...), IgnoreErrors(hos.ErrExist))...)
}

// EditUser updates a user's public key or disables the account (admin only)
func (c *Client) EditUser(ctx context.Context, user *hos.User, opts ...Options) error {
	if c.user != constant.AdminUser {
		return hos.ErrAdminOnly
	}
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return err
	}

	b, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("encoding json failed: %w", err)
	}

	responses := c.doP(ctx, "POST", constant.UserAPIPrefix, c.servers, append(modifiers(opts...), jsonBody(b))...)
	return handleErrors(responses, errHandlers(opts...)...)
}

// ListUsers returns all registered users (admin only)
func (c *Client) ListUsers(ctx context.Context, opts ...Options) ([]hos.User, error) {
	if c.user != constant.AdminUser {
		return nil, hos.ErrAdminOnly
	}
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return nil, err
	}

	responses := c.doP(ctx, "GET", constant.UserAPIPrefix, c.servers, modifiers(opts...)...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return merge[hos.User](responses)
}

// DeleteUser removes a user from the system (admin only, cannot delete admin or anonymous user)
func (c *Client) DeleteUser(ctx context.Context, uid string, opts ...Options) error {
	if c.user != constant.AdminUser {
		return hos.ErrAdminOnly
	}

	// check uid if admin or anon fail
	if uid == adminID || uid == anonID {
		return hos.ErrNotAllowed
	}

	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return err
	}

	users, err := c.ListUsers(ctx, append(opts, IgnoreErrors(hos.ErrNotExist))...)
	if err != nil {
		return err
	}
	username := ""
	for _, u := range users {
		if u.ID == uid {
			username = u.Name
		}
	}
	if username == "" {
		return hos.ErrNotExist
	}

	modifiersList := append(opts, IgnoreErrorsExcept(), OnBehalf(username))
	pools, err := c.ListPools(ctx, modifiersList...)
	if err != nil {
		return err
	}
	if poolCount := len(pools); poolCount != 0 {
		return fmt.Errorf("user %s has %d pools, %w", username, poolCount, hos.ErrNotEmpty)
	}

	responses := c.doP(ctx, "DELETE", path.Join(constant.UserAPIPrefix, uid), c.servers, modifiers(opts...)...)
	return handleErrors(responses, errHandlers(opts...)...)
}
