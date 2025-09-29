// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/server"
	"github.com/google/go-cmp/cmp"
)

func TestClient_Health(t *testing.T) {
	ts := newTestServer(".health", t, createUser(user1), createUser(user2, server1, server3))

	tests := []struct {
		c       *Client
		wantErr error
		name    string
	}{
		{
			name: "admin user",
			c:    ts.C(adminUser),
		},
		{
			name: "normal user",
			c:    ts.C(user1),
		},
		{
			name: "partial user",
			c:    ts.C(user2),
		},
		{
			name: "not exist user",
			c:    ts.C(user3),
		},
		{
			name: "anonymous user",
			c:    ts.C(anonUser),
		},
		{
			name:    "some of the servers are unavailable",
			c:       ts.C(user1, server3, server4),
			wantErr: hos.ErrConnectionFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rip, err := tt.c.Health(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.Health() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(rip, "127.0.0.1"); tt.wantErr == nil && diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestClient_GetServerInfo(t *testing.T) {
	ts := newTestServer(".server_info", t, createUser(user1), createUser(user2, server1, server3))

	tests := []struct {
		c       *Client
		wantErr error
		name    string
	}{
		{
			name: "get full server info",
			c:    ts.C(user1, server2, server3),
		},
		{
			name:    "get server info including unreachable server",
			c:       ts.C(user1, server2, server3, server4),
			wantErr: hos.ErrConnectionFailure,
		},
		{
			name:    "get full server info with partial user",
			c:       ts.C(user2),
			wantErr: hos.ErrNotAuthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.c.GetServerInfo(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetServerInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestClient_ServerConfig(t *testing.T) {
	// store and restore the port helper after the test
	portB4 := port
	t.Cleanup(func() {
		port = portB4
	})
	// assign a new port helper so we can make sure to get the ports
	// in the expected result
	port = &tests.Port{Val: 6000}

	ts := newTestServer(".config", t, createUser(user1), createUser(user2, server1, server3))

	tests := []struct {
		c       *Client
		wantErr error
		name    string
	}{
		{
			name: "admin user",
			c:    ts.C(adminUser),
		},
		{
			name: "normal user",
			c:    ts.C(user1),
		},
		{
			name:    "partial user",
			c:       ts.C(user2),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name:    "anonymous user",
			c:       ts.C(anonUser),
			wantErr: hos.ErrNotAuthorized,
		},
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get the working dir: %s", err)
	}

	exp := map[string]server.Config{
		"127.0.0.1:6001": {
			RootDir:  filepath.Join(wd, ".config", "server1"),
			Address:  "127.0.0.1:6001",
			LogLevel: "none",
		},
		"127.0.0.1:6002": {
			RootDir:  filepath.Join(wd, ".config", "server2"),
			Address:  "127.0.0.1:6002",
			LogLevel: "none",
		},
		"127.0.0.1:6003": {
			RootDir:  filepath.Join(wd, ".config", "server3"),
			Address:  "127.0.0.1:6003",
			LogLevel: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confMap, err := tt.c.ServerConfig(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.ServerConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(confMap, exp); tt.wantErr == nil && diff != "" {
				t.Error(diff)
			}
		})
	}
}
