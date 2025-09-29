// SPDX-License-Identifier: MIT

package server

import (
	"io"
	"net/http"
	"path"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestServer_Pool(t *testing.T) {
	srv := newTestServer(".test_pool", t, user1, user2)

	// we will not create user3 on the server
	tc := newTestClient(srv, t, adminUser, user1, user2, user3)

	tests := []struct {
		pool      *hos.Pool
		wantPool  *hos.Pool
		name      string
		user      string
		method    string
		path      string
		wantPools []hos.Pool
		wantCode  int
	}{
		// create pool1 with user1 client
		{
			name:     "crt1",
			user:     user1,
			method:   put,
			pool:     tests.Pool(pool1),
			wantCode: 201,
			wantPool: tests.Pool(pool1, tests.UserID(user1ID), tests.RepCount(1)),
		},
		// create pool1 again with user1 client, already exists
		{
			name:     "crt2",
			user:     user1,
			method:   put,
			pool:     tests.Pool(pool1),
			wantCode: 409,
		},
		// create pool2 with admin client, not permitted
		{
			name:     "crt3",
			user:     adminUser,
			method:   put,
			pool:     tests.Pool(pool1),
			wantCode: 401,
		},
		// get pool1 with user1 client
		{
			name:     "get1",
			user:     user1,
			method:   head,
			path:     tests.PID(user1ID, pool1),
			wantCode: 204,
			wantPool: tests.Pool(pool1, tests.UserID(user1ID), tests.RepCount(1)),
		},
		// get pool1 with user2 client, not authz
		{
			name:     "get2",
			user:     user2,
			method:   head,
			path:     tests.PID(user1ID, pool1),
			wantCode: 403,
		},
		// get pool1 with admin client, not authz
		{
			name:     "get3",
			user:     adminUser,
			method:   head,
			path:     tests.PID(user1ID, pool1),
			wantCode: 401,
		},
		// create pool2 give read perm to everyone with user1 client
		{
			name:     "crt4",
			user:     user1,
			method:   put,
			pool:     tests.Pool(pool2, tests.Perms(everyone, read)),
			wantCode: 201,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(everyone, read),
			),
		},
		// get pool2 with user2 client
		{
			name:     "get4",
			user:     user1,
			method:   head,
			path:     tests.PID(user1ID, pool2),
			wantCode: 204,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(everyone, read),
			),
		},
		// get pool2 with anon user client
		{
			name:     "get5",
			user:     anonUser,
			method:   head,
			path:     tests.PID(user1ID, pool2),
			wantCode: 204,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(everyone, read),
			),
		},
		// get pool2 with admin client, not authz
		{
			name:     "get6",
			user:     adminUser,
			method:   head,
			path:     tests.PID(user1ID, pool2),
			wantCode: 401,
		},
		// get pool2 with user3 client, user3 is not exists
		{
			name:     "get7",
			user:     user3,
			method:   head,
			path:     tests.PID(user1ID, pool2),
			wantCode: 401,
		},
		// delete pool2 with user2 client, not authz
		{
			name:     "del1",
			user:     user2,
			method:   del,
			path:     tests.PID(user1ID, pool2),
			wantCode: 403,
		},
		// delete pool2 with anon user client, not authz
		{
			name:     "del2",
			user:     anonUser,
			method:   del,
			path:     tests.PID(user1ID, pool2),
			wantCode: 403,
		},
		// edit pool2 with user1 client
		{
			name:     "edit1",
			user:     user1,
			method:   post,
			path:     tests.PID(user1ID, pool2),
			pool:     tests.Pool(pool2, tests.Labels("L", "K"), tests.Perms("!*", read)),
			wantCode: 204,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Labels("L", "K"),
			),
		},
		// delete pool2 with user1 client
		{
			name:     "del3",
			user:     user1,
			method:   del,
			path:     tests.PID(user1ID, pool2),
			wantCode: 204,
		},
		// delete pool2 again with user1 client, not found
		{
			name:     "del4",
			user:     user1,
			method:   del,
			path:     tests.PID(user1ID, pool2),
			wantCode: 404,
		},
		// delete pool1 with admin client, not allowed
		{
			name:     "del5",
			user:     adminUser,
			method:   del,
			path:     tests.PID(user1ID, pool1),
			wantCode: 401,
		},
		// create pool2 give write perm to user2 with user1 client
		{
			name:     "crt5",
			user:     user1,
			method:   put,
			pool:     tests.Pool(pool2, tests.Perms(user2, write)),
			wantCode: 201,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(user2, write),
			),
		},
		// list pools with user1 client
		{
			name:     "list1",
			user:     user1,
			method:   get,
			wantCode: 200,
			wantPools: []hos.Pool{
				*tests.Pool(
					pool1,
					tests.UserID(user1ID),
					tests.RepCount(1),
				),
				*tests.Pool(
					pool2,
					tests.UserID(user1ID),
					tests.RepCount(1),
					tests.Perms(user2, write),
				),
			},
		},
		// list pools with user2 client
		{
			name:      "list2",
			user:      user2,
			method:    get,
			wantCode:  200,
			wantPools: []hos.Pool{},
		},
		// edit pool2 with user2 client
		{
			name:     "edit2",
			user:     user2,
			method:   post,
			path:     tests.PID(user1ID, pool2),
			pool:     tests.Pool(pool2, tests.Labels("Z", "X")),
			wantCode: 403,
		},
		// edit pool2 with admin client
		{
			name:     "edit3",
			user:     adminUser,
			method:   post,
			path:     tests.PID(user1ID, pool2),
			pool:     tests.Pool(pool2, tests.Labels("Z", "X")),
			wantCode: 401,
		},
		// edit pool2 with user1 client
		{
			name:     "edit4",
			user:     user1,
			method:   post,
			path:     tests.PID(user1ID, pool2),
			pool:     tests.Pool(pool2, tests.Perms(everyone, write, "!"+user2, read)),
			wantCode: 204,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(everyone, write),
			),
		},
		// delete pool2 with user2 client, not authz
		{
			name:     "del6",
			user:     user2,
			method:   del,
			path:     tests.PID(user1ID, pool2),
			wantCode: 403,
		},
		// create pool3 link to pool2 with user2 client
		{
			name:   "crt6",
			user:   user2,
			method: put,
			pool: tests.Pool(
				pool3,
				tests.RepCount(1),
				tests.Labels("A", "B"),
				tests.Perms(everyone, read),
				tests.Linked(user1ID, pool2),
			),
			wantCode: 201,
			wantPool: tests.Pool(
				pool3,
				tests.UserID(user2ID),
				tests.Linked(user1ID, pool2),
			),
		},
		// create pool4 link to pool2 with user2 client, link to link not allowed
		{
			name:   "crt7",
			user:   user2,
			method: put,
			pool: tests.Pool(
				pool4,
				tests.Linked(user2ID, pool3),
			),
			wantCode: 444,
		},
		// create pool4 link to pool2 (user2) with user2 client, link to not exist pool fails
		{
			name:   "crt8",
			user:   user2,
			method: put,
			pool: tests.Pool(
				pool4,
				tests.Linked(user2ID, pool2),
			),
			wantCode: 404,
		},
		// get pool3 with admin client, not allowed
		{
			name:     "get8",
			user:     adminUser,
			method:   head,
			path:     tests.PID(user2ID, pool3),
			wantCode: 401,
		},
		// get pool3 with user1 client, only owner is allowed
		{
			name:     "get9",
			user:     user1,
			method:   head,
			path:     tests.PID(user2ID, pool3),
			wantCode: 444,
		},
		// get pool3 with user2 client, not allowed
		{
			name:     "get10",
			user:     user2,
			method:   head,
			path:     tests.PID(user2ID, pool3),
			wantCode: 204,
			wantPool: tests.Pool(
				pool2,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Perms(everyone, write),
			),
		},
		// list pools with user2 client
		{
			name:     "list3",
			user:     user2,
			method:   get,
			wantCode: 200,
			wantPools: []hos.Pool{
				*tests.Pool(pool3, tests.UserID(user2ID), tests.Linked(user1ID, pool2)),
			},
		},
		// delete pool3 with user1 client, not authz
		{
			name:     "del7",
			user:     user1,
			method:   del,
			path:     tests.PID(user2ID, pool3),
			wantCode: 403,
		},
		// delete pool3 with user2 client
		{
			name:     "del8",
			user:     user2,
			method:   del,
			path:     tests.PID(user2ID, pool3),
			wantCode: 204,
		},
		// create /pool1 with user1 client, invalid pool name
		{
			name:     "crt9",
			user:     user1,
			method:   put,
			pool:     tests.Pool("/p"),
			wantCode: 400,
		},
		// create pool3 with user1 client which has an attr
		{
			name:     "crt10",
			user:     user1,
			method:   put,
			pool:     tests.Pool(pool3, tests.Attrs("color", "red")),
			wantCode: 201,
			wantPool: tests.Pool(
				pool3,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Attrs("color", "red"),
			),
		},
		// edit pool3 with user1 client, with an existing attr
		{
			name:     "edit5",
			user:     user1,
			method:   post,
			path:     tests.PID(user1ID, pool3),
			pool:     tests.Pool(pool3, tests.Attrs("color", "red", "season", "winter")),
			wantCode: 400,
		},
		// edit pool3 with user1 client, with a new attr
		{
			name:     "edit6",
			user:     user1,
			method:   post,
			path:     tests.PID(user1ID, pool3),
			pool:     tests.Pool(pool3, tests.Attrs("season", "winter")),
			wantCode: 204,
			wantPool: tests.Pool(
				pool3,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Attrs("color", "red", "season", "winter"),
			),
		},
		// create pool4 with user1 client
		{
			name:     "crt11",
			user:     user1,
			method:   put,
			pool:     tests.Pool(pool4, tests.Encrypted()),
			wantCode: 201,
			wantPool: tests.Pool(
				pool4,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Encrypted(),
			),
		},
		// edit pool4 with user1 client, add an attr
		{
			name:     "edit7",
			user:     user1,
			method:   post,
			path:     tests.PID(user1ID, pool4),
			pool:     tests.Pool(pool4, tests.Attrs("color", "green")),
			wantCode: 204,
			wantPool: tests.Pool(
				pool4,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Encrypted(),
				tests.Attrs("color", "green"),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.pool, nil)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				bb, _ := io.ReadAll(rsp.Body)
				t.Errorf("Expected %s(%d), got %s(%d), error: %s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode,
					string(bb),
				)
			}

			if tt.wantPool != nil {
				got, err := parseResponse[hos.Pool](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(&got, tt.wantPool,
					cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}

			if tt.wantPools != nil {
				got, err := parseResponse[[]hos.Pool](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(got, tt.wantPools,
					cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestServer_PoolObject(t *testing.T) {
	srv := newTestServer(".test_pool_obj", t, user1)

	tc := newTestClient(srv, t, adminUser, user1)

	tests := []struct {
		data     any
		wantPool *hos.Pool
		file     string
		name     string
		user     string
		method   string
		path     string
		wantCode int
	}{
		// create pool1 with user1 client
		{
			name:     "crt1",
			user:     user1,
			method:   put,
			data:     tests.Pool(pool1),
			wantCode: 201,
			wantPool: tests.Pool(
				pool1,
				tests.UserID(user1ID),
				tests.RepCount(1),
			),
		},
		// create test.jpg  with user1 client
		{
			name:     "crt2",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			data:     tests.FileJPG.Copy(),
			wantCode: 201,
		},
		// get pool1 with user1 client
		{
			name:     "get1",
			user:     user1,
			method:   head,
			path:     tests.PID(user1ID, pool1),
			wantCode: 204,
			wantPool: tests.Pool(
				pool1,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Size(1024),
				tests.ObjCount(1),
				tests.Hash("1f7ac931d16eccc42e958c67cc7a23211acfc8d3ee054223dbefb417d99ede9c"),
			),
		},
		// create test.dat  with user1 client
		{
			name:     "crt3",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			data:     tests.FileAVI.Copy(),
			wantCode: 201,
		},
		// get pool1 with user1 client
		{
			name:     "get2",
			user:     user1,
			method:   head,
			path:     tests.PID(user1ID, pool1),
			wantCode: 204,
			wantPool: tests.Pool(
				pool1,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Size(3072),
				tests.ObjCount(2),
				tests.Hash("b56b8d5b26ec3b36cd2bbd0ab9989cbb173507abea1f86af1c87aa2a9b1356bd"),
			),
		},
		// delete test.jpg with user1 client
		{
			name:     "del1",
			user:     user1,
			method:   del,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			wantCode: 204,
		},
		// get pool1 with user1 client
		{
			name:     "get3",
			user:     user1,
			method:   head,
			path:     tests.PID(user1ID, pool1),
			wantCode: 204,
			wantPool: tests.Pool(
				pool1,
				tests.UserID(user1ID),
				tests.RepCount(1),
				tests.Size(2048),
				tests.ObjCount(1),
				tests.Hash("de20f50d74267f27985f427e2ccf722db9a4e5dc23c2eee653f7b63ce087952a"),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.data, nil)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if tt.wantPool != nil {
				got, err := parseResponse[hos.Pool](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(&got, tt.wantPool,
					cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestServer_PoolListFilters(t *testing.T) {
	srv := newTestServer(".test_pool_filter", t, user1)

	tc := newTestClient(srv, t, adminUser, user1)

	pools := []hos.Pool{
		*tests.Pool("abc", tests.UserID(user1ID), tests.RepCount(1), tests.Labels("num", "even")),
		*tests.Pool("abd", tests.UserID(user1ID), tests.RepCount(1), tests.Labels("num", "odd")),
		*tests.Pool("abf", tests.UserID(user1ID), tests.RepCount(1), tests.Labels("num", "even")),
		*tests.Pool("acd", tests.UserID(user1ID), tests.RepCount(1), tests.Labels("num", "odd")),
		*tests.Pool("acf", tests.UserID(user1ID), tests.RepCount(1), tests.Labels("num", "even")),
	}

	aa := []any{}
	for _, p := range pools {
		aa = append(aa, &p)
	}
	create(tc, t, aa...)

	tests := []struct {
		wantOpts  *filter.Headers
		opts      *filter.Headers
		name      string
		user      string
		wantPools []hos.Pool
		wantCode  int
	}{
		// get pools starts with a
		{
			name:     "get1",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "a",
			},
			wantPools: pools,
		},
		// get pools starts with ab
		{
			name:     "get2",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ab",
			},
			wantPools: pools[:3],
		},
		// get pools starts with ac
		{
			name:     "get3",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ac",
			},
			wantPools: pools[3:],
		},
		// get pools starts with a, range 2
		{
			name:     "get4",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "a",
				Range:      []int{1, 2},
			},
			wantPools: pools[1:3],
		},
		// get pools starts with ac, range 3
		{
			name:     "get5",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ac",
				Range:      []int{0, 3},
			},
			wantOpts: &filter.Headers{
				Range:      []int{0, 2},
				NamePrefix: "ac",
			},
			wantPools: pools[3:],
		},
		// get pools starts with ab, label num=even, and range 5
		{
			name:     "get6",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ab",
				Range:      []int{0, 5},
				Labels: []filter.Label{
					{Key: "num", Value: "even", Equal: true},
				},
			},
			wantOpts: &filter.Headers{
				Range: []int{0, 2}, NamePrefix: "ab",
				Labels: []filter.Label{
					{Key: "num", Value: "even", Equal: true},
				},
			},
			wantPools: []hos.Pool{pools[0], pools[2]},
		},
		// get pools starts with ac, label num equal odd and not equal odd
		{
			name:     "get7",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ac",
				Labels: []filter.Label{
					{Key: "num", Value: "even", Equal: false},
					{Key: "num", Value: "even", Equal: true},
				},
			},
			wantPools: []hos.Pool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsp, err := tc.do(tt.user, get, constant.APIPrefix, nil, header.Serialize(tt.opts))
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if tt.wantPools != nil {
				got, err := parseResponse[[]hos.Pool](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(got, tt.wantPools,
					cmpopts.IgnoreFields(hos.Pool{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}

			if tt.wantOpts == nil {
				tt.wantOpts = tt.opts
			}

			opts, _ := header.Parse[filter.Headers](rsp.Header)
			if diff := cmp.Diff(opts, tt.wantOpts); diff != "" {
				t.Error(diff)
			}
		})
	}
}
