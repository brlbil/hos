// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/id"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestClient_CreateUser(t *testing.T) {
	ts := newTestServer(".create_user", t)

	testCases := []struct {
		c       *Client
		user    *hos.User
		wantErr error
		name    string
	}{
		{
			name:    "without admin registering",
			c:       ts.C(adminUser),
			user:    &hos.User{Name: adminUser},
			wantErr: hos.ErrNotInitialized,
		},
		{
			name:    "create admin user",
			c:       ts.C(adminUser),
			user:    tests.User(adminUser),
			wantErr: hos.ErrNotAllowed,
		},
		{
			name:    "only admin user can use this func",
			c:       ts.C(user1),
			user:    tests.User(user2),
			wantErr: hos.ErrAdminOnly,
		},
		{
			name: "create user1",
			c:    ts.C(adminUser),
			user: tests.User(user1),
		},
		{
			name:    "if not all servers available fails",
			c:       ts.C(adminUser, server1, server4),
			user:    tests.User(user2),
			wantErr: hos.ErrConnectionFailure,
		},
		{
			name:    "anon user cannot be created",
			c:       ts.C(adminUser),
			user:    tests.User(anonUser),
			wantErr: hos.ErrNotAllowed,
		},
	}

	for i, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.CreateUser(context.Background(), tt.user); !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.CreateUser() error = %v, wantErr %v", err, tt.wantErr)
			}

			if i == 0 {
				err := tt.c.EditUser(context.Background(), tests.User(adminUser))
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestClient_UpdateUser(t *testing.T) {
	ts := newTestServer(".update_user", t, createUser(user2))

	tests := []struct {
		c       *Client
		user    *hos.User
		wantErr error
		name    string
	}{
		{
			name:    "not exits",
			c:       ts.C(adminUser),
			user:    tests.User(user1),
			wantErr: hos.ErrNotExist,
		},
		{
			name: "add key to admin",
			c:    ts.C(adminUser),
			user: tests.User(adminUser, int(server2)),
		},
		{
			name:    "admin only",
			c:       ts.C(user2),
			user:    tests.User(user2, int(server2)),
			wantErr: hos.ErrAdminOnly,
		},
		{
			name: "add keys to user2",
			c:    ts.C(adminUser),
			user: tests.User(user2, int(server2)),
		},
		{
			name: "add/remove keys to user2",
			c:    ts.C(adminUser),
			user: tests.User(user2, -1, int(server3)),
		},
		{
			name:    "id and user name must match",
			c:       ts.C(adminUser),
			user:    &hos.User{Name: user2, ID: user1ID},
			wantErr: hos.ErrBadRequest,
		},
		{
			name:    "all servers must be available",
			c:       ts.C(adminUser, server1, server2, server4),
			user:    tests.User(user2, int(server2)),
			wantErr: hos.ErrConnectionFailure,
		},
		{
			name:    "updating anon user is not allowed",
			c:       ts.C(adminUser),
			user:    tests.User(anonUser),
			wantErr: hos.ErrNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.EditUser(context.Background(), tt.user); !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.UpdateUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_GetUsers(t *testing.T) {
	ts := newTestServer(".list_user", t, createUser(user1, server1, server3))

	tests := []struct {
		c       *Client
		name    string
		wantErr error
		want    []hos.User
	}{
		{
			name: "list users",
			c:    ts.C(adminUser),
			want: []hos.User{*tests.User(adminUser), *tests.User(user1)},
		},
		{
			name:    "only admin user",
			c:       ts.C(user1),
			wantErr: hos.ErrAdminOnly,
		},
		{
			name:    "partial servers regular user",
			c:       ts.C(user1, server1),
			wantErr: hos.ErrAdminOnly,
		},
		{
			name:    "some of the servers are not available",
			c:       ts.C(adminUser, server1, server4),
			wantErr: hos.ErrConnectionFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.ListUsers(context.Background())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetUsers() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.User{})); diff != "" {
				t.Errorf("Client.GetUsers() %s", diff)
			}
		})
	}
}

func TestClient_DeleteUser(t *testing.T) {
	ts := newTestServer(".list_user", t, createUser(user1), createUser(user2, server1, server3))

	create[hos.Pool](t, ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(3)))

	tests := []struct {
		c       *Client
		wantErr error
		name    string
		uid     string
	}{
		{
			name:    "deleting admin user is not allowed",
			c:       ts.C(adminUser),
			uid:     id.Admin,
			wantErr: hos.ErrNotAllowed,
		},
		{
			name:    "deleting anon user is not allowed",
			c:       ts.C(adminUser),
			uid:     id.Anonymous,
			wantErr: hos.ErrNotAllowed,
		},
		{
			name:    "not exist user",
			c:       ts.C(adminUser),
			uid:     user3ID,
			wantErr: hos.ErrNotExist,
		},
		{
			name:    "partial user",
			c:       ts.C(adminUser),
			uid:     user2ID,
			wantErr: hos.ErrNotExist,
		},
		{
			name:    "delete user with existing pools, pool is partially created",
			c:       ts.C(adminUser),
			uid:     user1ID,
			wantErr: hos.ErrNotEmpty,
		},
		{
			name:    "oly admin use this api",
			c:       ts.C(user1),
			uid:     user1ID,
			wantErr: hos.ErrAdminOnly,
		},
		{
			name:    "some of the servers is unreachable",
			c:       ts.C(adminUser, server1, server4),
			uid:     user1ID,
			wantErr: hos.ErrConnectionFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.DeleteUser(context.Background(), tt.uid); !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.DeleteUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_GetUsage(t *testing.T) {
	ts := newTestServer(".delete_object", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1), tests.Pool(pool1, tests.RepCount(2)),
		ts.C(user1), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user2, server3), tests.Pool(pool3),
	)

	create[tests.File](t,
		ts.C(user1), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)),
		ts.C(user1), tests.FileCSV.Copy(tests.PoolID(user1ID, pool1)),
		ts.C(user2, server3), tests.FileCSV.Copy(tests.PoolID(user2ID, pool3)),
	)

	tests := []struct {
		c       *Client
		want    *hos.Usage
		name    string
		wantErr error
		opts    []Options
	}{
		{
			name: "get user1 usage",
			c:    ts.C(user1),
			want: &hos.Usage{Name: user1, Pools: 1, Object: 2, Size: 2072},
		},
		{
			name: "get user1 usage with impersonation",
			c:    ts.C(adminUser),
			opts: []Options{OnBehalf(user1)},
			want: &hos.Usage{Name: user1, Pools: 1, Object: 2, Size: 2072},
		},
		{
			name: "get user2 usage",
			c:    ts.C(user2, server1, server3),
			want: &hos.Usage{Name: user2, Pools: 1, Object: 1, Size: 24},
		},
		{
			name:    "get user2 usage on all servers",
			c:       ts.C(user2),
			wantErr: hos.ErrNotAuthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.GetUsage(context.Background(), tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetUsage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Client.GetUsage() %s", diff)
			}
		})
	}
}
