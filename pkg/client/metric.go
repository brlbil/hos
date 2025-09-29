// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"path"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/pkg/server"
)

// GetServerInfo returns disk capacity and load information for all servers
func (c *Client) GetServerInfo(ctx context.Context, opts ...Options) ([]hos.ServerInfo, error) {
	rsp := c.doP(ctx, "HEAD", constant.APIPrefix, c.servers, modifiers(opts...)...)
	if err := handleErrors(rsp, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	return convert(rsp)
}

// Health checks the health status of all configured servers
func (c *Client) Health(ctx context.Context, ehs ...ErrorHandler) (string, error) {
	rsp := c.doP(ctx, "GET", "/healthz", c.servers)
	if err := handleErrors(rsp, ehs...); err != nil {
		return "", err
	}
	buf, err := getOneBody(rsp)
	if err != nil {
		return "", err
	}
	s := string(buf)
	i := strings.Index(s, "RemoteAddr:")
	return strings.Trim(s[i+11:], "\n "), nil
}

// ServerConfig retrieves configuration settings from all servers (localhost clusters only)
func (c *Client) ServerConfig(ctx context.Context, ehs ...ErrorHandler) (map[string]server.Config, error) {
	rsp := c.doP(ctx, "GET", path.Join(constant.APIPrefix, "config"), c.servers)
	if err := handleErrors(rsp, ehs...); err != nil {
		return nil, err
	}
	return getServerConfig(rsp)
}
