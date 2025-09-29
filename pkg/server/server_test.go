// SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
)

func TestServerInfo(t *testing.T) {
	srv := newTestServer(".test_info", t, user1)

	tc := newTestClient(srv, t, adminUser, user1, user2)

	tests := []struct {
		name     string
		user     string
		wantCode int
	}{
		{
			name:     "admin",
			user:     adminUser,
			wantCode: 204,
		},
		{
			name:     "anon",
			user:     anonUser,
			wantCode: 204,
		},
		{
			name:     "user1",
			user:     user1,
			wantCode: 204,
		},
		{
			name:     "user2",
			user:     user2,
			wantCode: 401,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp, err := tc.do(tt.user, head, constant.APIPrefix, nil, nil)
			if err != nil {
				t.Error(err)
				return
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if rsp.StatusCode >= 400 {
				return
			}

			si, err := parseResponse[hos.ServerInfo](rsp)
			if err != nil {
				t.Error(err)
			}

			if si.Blocks == 0 || si.FreeDisk() == 0 {
				t.Errorf("expected non zero info %v", si)
			}
		})
	}
}

func TestServerConfig(t *testing.T) {
	srv := newTestServer(".test_config", t, user1)

	tc := newTestClient(srv, t, adminUser, user1)

	tests := []struct {
		name     string
		user     string
		wantCode int
	}{
		{
			name:     "admin",
			user:     adminUser,
			wantCode: 200,
		},
		{
			name:     "anon",
			user:     anonUser,
			wantCode: 401,
		},
		{
			name:     "user1",
			user:     user1,
			wantCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := path.Join(constant.APIPrefix, "config")
			rsp, err := tc.do(tt.user, get, p, nil, nil)
			if err != nil {
				t.Error(err)
				return
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if rsp.StatusCode >= 400 {
				return
			}

			conf, err := parseResponse[Config](rsp)
			if err != nil {
				t.Error(err)
			}

			wd, err := os.Getwd()
			if err != nil {
				t.Error(err)
			}

			expDir := filepath.Join(wd, ".test_config")
			if conf.RootDir != expDir {
				t.Errorf("expected RootDir %s, got %s", expDir, conf.RootDir)
			}

			if conf.LogLevel != "none" {
				t.Errorf("expected LogLevel none, got %s", conf.LogLevel)
			}
		})
	}
}
