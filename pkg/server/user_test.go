// SPDX-License-Identifier: MIT

package server

import (
	"io"
	"net/http"
	"path"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/id"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestServer_Users(t *testing.T) {
	srv := newTestServer(".test_user", t)

	tc := newTestClient(srv, t, adminUser, user1)

	tests := []struct {
		userData  *hos.User
		name      string
		user      string
		method    string
		path      string
		wantUsers []hos.User
		wantCode  int
	}{
		// create admin with admin client, admin token is not set yet
		{
			name:     "crt1",
			user:     adminUser,
			method:   put,
			userData: tests.User(adminUser),
			wantCode: 434,
		},
		// list all users with admin client on not initialized cluster
		{
			name:     "get0",
			user:     adminUser,
			method:   get,
			wantCode: 434,
		},
		// create user1 with user1 client,user1 is not exist and user1 is not allowed
		{
			name:     "crt2",
			user:     user1,
			method:   put,
			userData: tests.User(user1),
			wantCode: 401,
		},
		// update user1 with user1 client, user1 is not exist and user1 is not allowed
		{
			name:     "upt1",
			user:     user1,
			method:   post,
			userData: tests.User(user1),
			wantCode: 401,
		},
		// update user1 with admin client, admin token is not set yet
		{
			name:     "upt2",
			user:     adminUser,
			method:   post,
			userData: tests.User(user1),
			wantCode: 444,
		},
		// update admin with admin client
		{
			name:     "upt3",
			user:     adminUser,
			method:   post,
			userData: tests.User(adminUser),
			wantCode: 204,
		},
		// update user1 with admin client, user1 is not exist
		{
			name:     "upt4",
			user:     adminUser,
			method:   post,
			userData: tests.User(user1),
			wantCode: 404,
		},
		// create user1 with admin client
		{
			name:     "crt3",
			user:     adminUser,
			method:   put,
			userData: tests.User(user1),
			wantCode: 201,
		},
		// create user1 with admin client, user is already exist
		{
			name:     "crt4",
			user:     adminUser,
			method:   put,
			userData: tests.User(user1),
			wantCode: 409,
		},
		// create admin with admin client, this is not allowed
		{
			name:     "crt5",
			user:     adminUser,
			method:   put,
			userData: tests.User(adminUser),
			wantCode: 444,
		},
		// create anonymous with admin client, this is not allowed
		{
			name:     "crt6",
			user:     adminUser,
			method:   put,
			userData: tests.User(anonUser),
			wantCode: 444,
		},
		// create a user without any keys with admin client, this is not allowed
		{
			name:     "crt7",
			user:     adminUser,
			method:   put,
			userData: tests.User("no_key"),
			wantCode: 400,
		},
		// create user2 with admin client, multi key on creation is not allowed
		{
			name:     "crt8",
			user:     adminUser,
			method:   put,
			userData: tests.User(user2, 0, 1),
			wantCode: 400,
		},
		// create user2 with admin client
		{
			name:     "crt9",
			user:     adminUser,
			method:   put,
			userData: tests.User(user2),
			wantCode: 201,
		},
		// create a user with admin client, invalid user name
		{
			name:     "crt10",
			user:     adminUser,
			method:   put,
			userData: tests.User("1" + user1),
			wantCode: 400,
		},
		// update user2 with wrong ID with admin client, this is not allowed
		{
			name:     "upt5",
			user:     adminUser,
			method:   post,
			userData: &hos.User{Name: user2, ID: id.Gen(user1)},
			wantCode: 400,
		},
		// update anonymous with admin client, this is not allowed
		{
			name:     "upt6",
			user:     adminUser,
			method:   post,
			userData: tests.User(anonUser),
			wantCode: 444,
		},
		// add key2 twice to user2 with admin client
		{
			name:     "upt7",
			user:     adminUser,
			method:   post,
			userData: tests.User(user2, 1, 1),
			wantCode: 204,
		},
		// add key1 and key2 user2 with admin client
		{
			name:     "upt8",
			user:     adminUser,
			method:   post,
			userData: tests.User(user2, 0, 1),
			wantCode: 204,
		},
		// remove not exist key3 from user2 with admin client, not exist
		{
			name:     "upt9",
			user:     adminUser,
			method:   post,
			userData: tests.User(user2, -2),
			wantCode: 400,
		},
		// add key3 and remove key2 from user2 with admin client
		{
			name:     "upt10",
			user:     adminUser,
			method:   post,
			userData: tests.User(user2, 2, -1),
			wantCode: 204,
		},
		// list all users with admin client
		{
			name:     "get1",
			user:     adminUser,
			method:   get,
			userData: nil,
			wantUsers: []hos.User{
				*tests.User(adminUser),
				*tests.User(user1),
				*tests.User(user2, 0, 2),
			},
			wantCode: 200,
		},
		// list all users with user1 client, not authz
		{
			name:     "get2",
			user:     user1,
			method:   get,
			wantCode: 401,
		},
		// delete anonymous with admin client, not allowed
		{
			name:     "del1",
			user:     adminUser,
			path:     id.Anonymous,
			method:   del,
			wantCode: 444,
		},
		// delete admin with admin client, not allowed
		{
			name:     "del2",
			user:     adminUser,
			path:     id.Admin,
			method:   del,
			wantCode: 444,
		},
		// delete user1 with user1 client, not authz
		{
			name:     "del3",
			user:     user1,
			path:     user1ID,
			method:   del,
			wantCode: 401,
		},
		// delete user2 with admin client
		{
			name:     "del4",
			user:     adminUser,
			path:     user2ID,
			method:   del,
			wantCode: 204,
		},
		// delete user2 with admin client, not exist
		{
			name:     "del5",
			user:     adminUser,
			path:     user2ID,
			method:   del,
			wantCode: 404,
		},
		// list all users with admin client
		{
			name:   "get3",
			user:   adminUser,
			method: get,
			wantUsers: []hos.User{
				*tests.User(adminUser),
				*tests.User(user1),
			},
			wantCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.UserAPIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.userData, nil)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				msg, _ := io.ReadAll(rsp.Body)
				t.Errorf("Expected %s(%d), got %s(%d), msg: %s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode, string(msg))
			}

			got, err := parseResponse[[]hos.User](rsp)
			if err != nil {
				t.Error(err)
			}

			if diff := cmp.Diff(got, tt.wantUsers, cmpopts.IgnoreUnexported(hos.User{})); diff != "" {
				t.Error(diff)
			}
		})
	}
}
