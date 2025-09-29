// SPDX-License-Identifier: MIT

package server

import (
	"io"
	"net/http"
	"path"
	"testing"

	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/id"
	"github.com/google/go-cmp/cmp"
)

func TestServer_HealthCert(t *testing.T) {
	srv := newTestServer(".test_health", t)

	tc := newTestClient(srv, t, user1)

	tests := []struct {
		name     string
		path     string
		data     []byte
		wantCode int
	}{
		{
			name:     "ca",
			path:     "/ca",
			wantCode: 200,
			data:     srv.caCert,
		},
		{
			name:     "health",
			path:     "/healthz",
			wantCode: 200,
			data:     []byte("Status:OK RemoteAddr:127.0.0.1\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp, err := tc.do("", get, tt.path, nil, nil)
			if err != nil {
				t.Error(err)
				return
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, rsp.StatusCode)
				return
			}

			ca, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Error(err)
				return
			}

			if diff := cmp.Diff(ca, tt.data); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestServer_Auth(t *testing.T) {
	srv := newTestServer(".test_auth", t)

	tc := newTestClient(srv, t, adminUser, user1, user2)

	tests := []struct {
		data     any
		header   map[string]string
		name     string
		user     string
		method   string
		path     string
		wantCode int
	}{
		// update admin users publickey with admin client
		{
			name:     "uptUser1",
			user:     adminUser,
			method:   post,
			path:     "user",
			data:     tests.User(adminUser),
			wantCode: 204,
		},
		// create user1 with admin client
		{
			name:     "crtUser1",
			user:     adminUser,
			method:   put,
			path:     "user",
			data:     tests.User(user1),
			wantCode: 201,
		},
		// create user2 with admin client
		{
			name:     "crtUser2",
			user:     adminUser,
			method:   put,
			path:     "user",
			data:     tests.User(user2),
			wantCode: 201,
		},
		// create pool1 give write access to user2 with user1 client
		{
			name:   "crtPool1",
			user:   user1,
			method: put,
			data: tests.Pool(
				pool1,
				tests.Perms(user2, write),
				tests.Labels("L1", "K1"),
			),
			wantCode: 201,
		},
		// create pool2 give read access to everyone with user1 client
		{
			name:     "crtPool2",
			user:     user1,
			method:   put,
			data:     tests.Pool(pool2, tests.Perms(everyone, write)),
			wantCode: 201,
		},
		// create pool3 as link to pool2 with user2 client
		{
			name:     "crtPool3",
			user:     user2,
			method:   put,
			data:     tests.Pool(pool3, tests.Linked(user1ID, pool2)),
			wantCode: 201,
		},
		// create pool4 with anonymous client, not authorized
		{
			name:     "crtPool4",
			user:     anonUser,
			method:   put,
			data:     tests.Pool(pool4),
			wantCode: 401,
		},
		// create object test.jpg in pool1 with user2, user2 has write permissions to pool1
		{
			name:     "crtObj1",
			user:     user2,
			method:   put,
			path:     id.Gen(user1ID, pool1),
			data:     tests.FileJPG.Copy(),
			wantCode: 201,
		},
		// create object test.data in pool2 with user1, user1 is owner
		{
			name:     "crtObj2",
			user:     user1,
			method:   put,
			path:     id.Gen(user1ID, pool2),
			data:     tests.FileAVI.Copy(),
			wantCode: 201,
		},
		// create object test.jpg in pool1 with admin
		{
			name:     "crtObj3",
			user:     adminUser,
			method:   put,
			path:     id.Gen(user1ID, pool1),
			data:     tests.FileJPG.Copy(),
			wantCode: 401,
		},
		// get obj1 with admin client, admin cannot read an object
		{
			name:     "getObj1",
			user:     adminUser,
			method:   head,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			wantCode: 401,
		},
		// get obj1 with admin client on behalf of user1
		{
			name:     "getObj2",
			user:     adminUser,
			method:   head,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			header:   map[string]string{header.OnBehalf: user1},
			wantCode: 204,
		},
		// get obj2 with admin client, admin cannot read an object
		{
			name:     "getObj3",
			user:     adminUser,
			method:   head,
			path:     tests.OLinkPath(user2ID, pool3, tests.OID(user1ID, pool2, tests.AVI)),
			wantCode: 401,
		},
		// get obj2 with admin client on behalf of user2
		{
			name:     "getObj4",
			user:     adminUser,
			method:   head,
			path:     tests.OLinkPath(user2ID, pool3, tests.OID(user1ID, pool2, tests.AVI)),
			header:   map[string]string{header.OnBehalf: user2},
			wantCode: 204,
		},
		// list objects in pool1 with admin client, not allowed
		{
			name:     "listObj1",
			user:     adminUser,
			method:   get,
			path:     id.Gen(user1ID, pool1),
			wantCode: 401,
		},
		// list objects in pool1 with admin client on behalf of user1
		{
			name:     "listObj2",
			user:     adminUser,
			method:   get,
			path:     id.Gen(user1ID, pool1),
			header:   map[string]string{header.OnBehalf: user1},
			wantCode: 200,
		},
		// list objects in pool1 with admin client, not allowed
		{
			name:     "listObj3",
			user:     adminUser,
			method:   get,
			path:     id.Gen(user2ID, pool3),
			wantCode: 401,
		},
		// list objects in pool1 with admin client on behalf of user1
		{
			name:     "listObj4",
			user:     adminUser,
			method:   get,
			path:     id.Gen(user2ID, pool3),
			header:   map[string]string{header.OnBehalf: user2},
			wantCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.data, tt.header)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}
		})
	}
}
