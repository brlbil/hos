// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"path"
	"slices"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/compare"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/id"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestClient_CreatePool(t *testing.T) {
	ts := newTestServer(".create_pool", t, createUser(user1), createUser(user2, server1, server3))

	tests := []struct {
		c        *Client
		pool     *hos.Pool
		wantPool *hos.Pool
		name     string
		wantErr  error
		opts     []Options
	}{
		{
			name:    "create pool1 with partially exist user",
			c:       ts.C(user2),
			pool:    tests.Pool(pool1),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name: "create pool1 on some of the servers, rc is bigger then available servers",
			c:    ts.C(user1, server1, server3),
			pool: tests.Pool(pool1, tests.RepCount(3)),
			wantPool: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.RepCount(3),
				tests.Hash("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			),
		},
		{
			name:    "create pool1 on all of the servers, with Encryption (should fail)",
			c:       ts.C(user1),
			pool:    tests.Pool(pool1, tests.RepCount(3), tests.Encrypted()),
			wantErr: hos.ErrBadRequest,
		},
		{
			name:    "create pool1 on all of the servers, with different label (should fail)",
			c:       ts.C(user1),
			pool:    tests.Pool(pool1, tests.RepCount(3), tests.Labels("K", "Z")),
			wantErr: hos.ErrBadRequest,
		},
		{
			name:    "create pool1 on all of the servers, with different label (should fail)",
			c:       ts.C(user1),
			pool:    tests.Pool(pool1, tests.RepCount(3), tests.Attrs("A", "B")),
			wantErr: hos.ErrBadRequest,
		},
		{
			name: "create pool1 on all of the servers again",
			c:    ts.C(user1),
			pool: tests.Pool(pool1, tests.RepCount(3)),
			wantPool: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.RepCount(3),
			),
		},
		{
			name:    "create pool2 on not reachable server",
			c:       ts.C(user1, server3, server4),
			pool:    tests.Pool(pool2, tests.Labels("X", "Y")),
			wantErr: hos.ErrConnectionFailure,
		},
		{
			name:    "create pool2 on with different config and with impersonation (should fail)",
			c:       ts.C(adminUser),
			pool:    tests.Pool(pool2, tests.Labels("K", "V")),
			opts:    []Options{OnBehalf(user1)},
			wantErr: hos.ErrBadRequest,
		},
		{
			name: "create pool2 on with impersonation",
			c:    ts.C(adminUser),
			pool: tests.Pool(pool2),
			opts: []Options{OnBehalf(user1)},
			wantPool: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Labels("X", "Y"),
				tests.Hash("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			),
		},
		{
			name: "create a link to pool2",
			c:    ts.C(user1),
			pool: tests.Pool(pool3, tests.Linked(user1ID, pool2)),
			wantPool: tests.Pool(pool3,
				tests.UserID(user1ID),
				tests.RepCount(0),
				tests.Linked(user1ID, pool2),
			),
		},
	}

	for m, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = m
			got, err := tt.c.CreatePool(context.Background(), tt.pool, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.CreatePool() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(got, tt.wantPool,
				cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
				t.Error(diff)
			}
			if tt.wantErr != nil {
				return
			}

			if _, err := tt.c.GetPool(context.Background(), id.Gen(tt.pool.Name), NoRedirect()); err == nil {
				t.Error("CreatePool get pool successful, expected failure")
			}
		})
	}
}

func TestClient_EditPool(t *testing.T) {
	ts := newTestServer(".edit_pool", t, createUser(user1), createUser(user2, server1, server3))

	create[hos.Pool](t,
		ts.C(user1), tests.Pool(pool1, tests.RepCount(3), tests.Labels("X", "Y")),
		ts.C(user1), tests.Pool(pool2, tests.RepCount(2), tests.Perms(user2, read)),
		ts.C(user1), tests.Pool(pool4, tests.RepCount(3), tests.Attrs("color", "green")),
		ts.C(user2, server1, server3), tests.Pool(pool3, tests.RepCount(4), tests.Linked(user1ID, pool1)),
	)

	tests := []struct {
		c       *Client
		p       *hos.Pool
		want    *hos.Pool
		name    string
		wantErr error
		opts    []Options
	}{
		{
			name:    "pool is nil",
			c:       ts.C(user1),
			wantErr: hos.ErrNotInitialized,
		},
		{
			name: "edit pool1",
			c:    ts.C(user1),
			p: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.Perms(everyone, write),
				tests.Labels("!X", "", "K", "V"),
			),
			want: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.RepCount(3),
				tests.Labels("K", "V"),
				tests.Perms(everyone, write),
				tests.Hash("cd372fb85148700fa88095e3492d3f9f5beb43e555e5ff26d95f5a6adc36f8e6"),
			),
		},
		{
			name: "only make changes on some of the servers, this will create drift",
			c:    ts.C(user1, server1, server3),
			p: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.Perms(everyone, read, "!"+user2, read),
			),
			want: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.RepCount(2),
				tests.Perms(everyone, read),
				tests.Hash("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			),
		},
		{
			name: "make changes on a pool that has drift",
			c:    ts.C(user1),
			p: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.Perms("!"+everyone, read),
			),
			wantErr: hos.ErrNotEqual,
		},
		{
			name: "make changes on a pool that has drift, with ignoring errors, and with impersonation",
			c:    ts.C(adminUser),
			p: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.Perms("!"+everyone, read),
			),
			opts: []Options{IgnoreErrors(hos.ErrNotEqual, hos.ErrBadRequest), OnBehalf(user1)},
			want: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.RepCount(2),
				tests.Hash("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			),
		},
		{
			name:    "operate on destination linked pool",
			c:       ts.C(user2, server1, server3),
			p:       tests.Pool(pool1, tests.UserID(user1ID)),
			wantErr: hos.ErrInsufficientPermissions,
		},
		{
			name: "change the link pool",
			c:    ts.C(user2, server1, server3),
			p: tests.Pool(pool3,
				tests.UserID(user2ID),
				tests.Linked(user1ID, pool2),
			),
			wantErr: hos.ErrNotAllowed,
		},
		{
			name: "add an attr on a pool that has none",
			c:    ts.C(user1),
			p: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.Attrs("space", "black"),
			),
			want: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.RepCount(3),
				tests.Attrs("space", "black"),
				tests.Labels("K", "V"),
				tests.Perms(everyone, write),
				tests.Hash("cd372fb85148700fa88095e3492d3f9f5beb43e555e5ff26d95f5a6adc36f8e6"),
			),
		},
		{
			name: "change an attr on some of the servers",
			c:    ts.C(user1, server1, server3),
			p: tests.Pool(pool4,
				tests.UserID(user1ID),
				tests.Attrs("color", "red"),
			),
			wantErr: hos.ErrBadRequest,
		},
		{
			name: "change an attr on some of the servers",
			c:    ts.C(user1, server1, server3),
			p: tests.Pool(pool4,
				tests.UserID(user1ID),
				tests.Attrs("color", "red"),
			),
			wantErr: hos.ErrBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.EditPool(context.Background(), tt.p, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.EditPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want,
				cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
				t.Errorf("Client.EditPool() %s", diff)
			}
		})
	}
}

func TestClient_GetPool(t *testing.T) {
	ts := newTestServer(".gel_pool", t, createUser(user1), createUser(user2))

	create[hos.Pool](t,
		ts.C(user1), tests.Pool(pool1, tests.RepCount(1), tests.Perms(user2, read)),
		ts.C(user1, server2), tests.Pool(pool2, tests.RepCount(2), tests.Labels("X", "Y")),
		ts.C(user1), tests.Pool(pool3, tests.RepCount(3), tests.Perms(user2, read)),
		ts.C(user2, server1, server3), tests.Pool(pool3, tests.RepCount(4), tests.Linked(user1ID, pool3)),
	)

	create[tests.File](t, ts.C(user1), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)))

	tests := []struct {
		c       *Client
		want    *hos.Pool
		name    string
		id      string
		wantErr error
		opts    []Options
	}{
		{
			name: "get pool from all servers",
			c:    ts.C(user1),
			id:   tests.PID(user1ID, pool1),
			want: tests.Pool(pool1,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(user2, read),
				tests.Size(2048),
				tests.ObjCount(1),
			),
		},
		{
			name:    "get partially created pool from all servers",
			c:       ts.C(user1),
			id:      tests.PID(user1ID, pool2),
			wantErr: hos.ErrNotExist,
		},
		{
			name: "get pool, ignore partially not exist and with impersonation",
			c:    ts.C(adminUser),
			id:   tests.PID(user1ID, pool2),
			opts: []Options{IgnoreErrors(hos.ErrNotExist), OnBehalf(user1)},
			want: tests.Pool(pool2,
				tests.UserID(user1ID),
				tests.RepCount(2),
				tests.Labels("X", "Y"),
			),
		},
		{
			name: "get pool from linked pool which is not exist on all the servers",
			c:    ts.C(user2),
			id:   tests.PID(user2ID, pool3),
			opts: []Options{IgnoreErrors(hos.ErrNotExist)},
			want: tests.Pool(pool3,
				tests.UserID(user1ID),
				tests.RepCount(3),
				tests.Perms(user2, read),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.GetPool(context.Background(), tt.id, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want,
				cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt", "Hash")); diff != "" {
				t.Errorf("Client.GetPool() %s", diff)
			}
		})
	}
}

func TestClient_ListPools(t *testing.T) {
	ts := newTestServer(".list_pool", t, createUser(user1), createUser(user2), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1), tests.Pool(pool1, tests.RepCount(3), tests.Perms(user2, read)),
		ts.C(user1, server1, server3), tests.Pool(pool2, tests.RepCount(2), tests.Labels("X", "Y")),
		ts.C(user2, server2), tests.Pool(pool3),
		ts.C(user2, server1, server3), tests.Pool(pool4, tests.RepCount(4), tests.Linked(user1ID, pool1)),
		ts.C(user3, server2), tests.Pool(pool5, tests.RepCount(1)),
		ts.C(user3, server3), tests.Pool(pool5, tests.RepCount(2)),
	)

	create[tests.File](t, ts.C(user1), tests.FileJPG.Copy(tests.PoolID(user1ID, pool1)))

	tests := []struct {
		c       *Client
		name    string
		wantErr error
		opts    []Options
		want    []hos.Pool
	}{
		{
			name: "list pools belong to user1",
			c:    ts.C(user1),
			want: []hos.Pool{
				*tests.Pool(pool1,
					tests.UserID(user1ID),
					tests.RepCount(3),
					tests.Perms(user2, read),
					tests.ObjCount(1),
					tests.Size(1024),
					tests.Hash("2507bf7c9e181a02fb447542baef75ff810c38b97f807a87899e3af4821e8134"),
				),
				*tests.Pool(pool2,
					tests.UserID(user1ID),
					tests.RepCount(2),
					tests.Labels("X", "Y"),
					tests.Hash("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
				),
			},
		},
		{
			name: "list pools belong to user2 with impersonation",
			c:    ts.C(adminUser),
			opts: []Options{OnBehalf(user2)},
			want: []hos.Pool{
				*tests.Pool(pool3,
					tests.UserID(user2ID),
					tests.RepCount(1),
				),
				*tests.Pool(pool4,
					tests.UserID(user2ID),
					tests.Linked(user1ID, pool1),
					tests.Hash("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
				),
			},
		},
		{
			name:    "list pools belong to user3, some pools are not equal",
			c:       ts.C(user3),
			wantErr: hos.ErrNotEqual,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.ListPools(context.Background(), tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.ListPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// depending on the pool availability it may or may not be sorted
			// so we are sorting here in order to test it
			slices.SortFunc(got, compare.Pool)

			if diff := cmp.Diff(got, tt.want,
				cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
				t.Errorf("Client.ListPool() %s", diff)
			}
		})
	}
}

func TestClient_DeletePool(t *testing.T) {
	ts := newTestServer(".gel_pool", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1), tests.Pool(pool1, tests.RepCount(3), tests.Perms(user2, read)),
		ts.C(user1, server1, server3), tests.Pool(pool2, tests.RepCount(2), tests.Labels("X", "Y")),
		ts.C(user2, server1, server3), tests.Pool(pool3, tests.RepCount(4), tests.Linked(user1ID, pool1)),
		ts.C(user3, server2), tests.Pool(pool4, tests.RepCount(1)),
		ts.C(user3, server3), tests.Pool(pool4, tests.RepCount(2)),
		ts.C(user3), tests.Pool(pool5, tests.RepCount(3)),
		ts.C(user2, server1, server3), tests.Pool(pool6, tests.Linked(user3ID, pool5)),
		ts.C(user1), tests.Pool(pool7, tests.RepCount(1)),
	)

	create[tests.File](t,
		ts.C(user1, server2), tests.FileCSV.Copy(tests.PoolID(user1ID, pool7)),
		ts.C(user3), tests.FileJPG.Copy(tests.PoolID(user3ID, pool5)),
	)

	tests := []struct {
		c         *Client
		wantErr   error
		name      string
		username  string
		id        string
		opts      []Options
		wantCount int
	}{
		{
			name:      "delete a pool on some of the servers",
			c:         ts.C(user1, server2),
			username:  user1,
			id:        tests.PID(user1ID, pool1),
			wantCount: 2,
		},
		{
			name:     "delete the same pool on all of the servers, again",
			c:        ts.C(user1),
			username: user1,
			id:       tests.PID(user1ID, pool1),
		},
		{
			name:      "delete a pool even in not running server",
			c:         ts.C(user1, server1, server2, server3, server4),
			username:  user1,
			id:        tests.PID(user1ID, pool2),
			wantErr:   hos.ErrConnectionFailure,
			wantCount: 2,
		},
		{
			name:     "delete the same pool again with impersonation",
			c:        ts.C(adminUser),
			username: user1,
			opts:     []Options{OnBehalf(user1)},
			id:       tests.PID(user1ID, pool2),
		},
		{
			name:     "delete a conflicting pool",
			c:        ts.C(user3),
			username: user3,
			id:       tests.PID(user3ID, pool4),
		},
		{
			name:     "delete a not exist pool",
			c:        ts.C(user3),
			username: user3,
			id:       tests.PID(user3ID, pool1),
			wantErr:  hos.ErrNotExist,
		},
		{
			name:     "delete a not empty pool",
			c:        ts.C(user3),
			username: user1,
			id:       tests.PID(user3ID, pool5),
			wantErr:  hos.ErrNotEmpty,
		},
		{
			name:     "delete a pool linked to a not empty pool",
			c:        ts.C(user2, server1, server3),
			username: user2,
			id:       tests.PID(user2ID, pool6),
		},
		{
			name:      "delete a not empty pool with not evenly distributed objects",
			c:         ts.C(user1),
			username:  user1,
			id:        tests.PID(user1ID, pool7),
			wantErr:   hos.ErrNotEmpty,
			wantCount: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.DeletePool(context.Background(), tt.id, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.DeletePool() error = %v, wantErr %v", err, tt.wantErr)
			}

			// let's verify the pool is not deleted
			// get the full client
			c := ts.C(tt.username)
			rsp := c.doP(context.Background(), "HEAD", path.Join(constant.APIPrefix, tt.id), c.servers)
			poolCount := 0
			for _, r := range rsp {
				if r.rsp != nil && r.rsp.StatusCode == 204 {
					poolCount++
				}
			}

			if tt.wantCount != poolCount {
				t.Errorf("Client.DeletePool() want pool count = %d, got %d", tt.wantCount, poolCount)
			}
		})
	}
}
