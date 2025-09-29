// SPDX-License-Identifier: MIT

package server

import (
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

func TestServer_Find(t *testing.T) {
	srv := newTestServer(".test_find", t, user1)

	tc := newTestClient(srv, t, adminUser, user1)

	create(tc, t,
		tests.Pool("abc", tests.UserID(user1ID)),
		tests.Pool("abd", tests.UserID(user1ID)),
		tests.Pool("cfd", tests.UserID(user1ID)),
		tests.FileJPG.Copy(tests.Name("adx"), tests.UserPoolID(user1ID, "abc")),
		tests.FileJPG.Copy(tests.Name("bkt"), tests.UserPoolID(user1ID, "abd")),
		tests.FileJPG.Copy(tests.Name("zxwk"), tests.UserPoolID(user1ID, "cfd")),
	)

	ob := []hos.Object{}
	for _, name := range []string{"abc", "abd", "cfd"} {
		ob = append(ob, hos.Object{ID: id.Gen(user1ID, name), Name: name})
	}
	for i, name := range []string{"adx", "bkt", "zxwk"} {
		ob = append(ob, hos.Object{ID: id.Gen(ob[i].ID, name), Name: name, PoolID: ob[i].ID})
	}

	tests := []struct {
		name     string
		user     string
		s        string
		want     []hos.Object
		wantCode int
	}{
		// get pools with admin user
		{
			name:     "get1",
			user:     adminUser,
			wantCode: 401,
		},
		// get pools with empty query
		{
			name:     "get2",
			user:     user1,
			wantCode: 400,
		},
		// get results with a
		{
			name:     "get3",
			user:     user1,
			s:        "a",
			want:     []hos.Object{ob[0], ob[1], ob[3]},
			wantCode: 200,
		},
		// get results with b
		{
			name:     "get4",
			user:     user1,
			s:        "d",
			want:     []hos.Object{ob[1], ob[2], ob[3]},
			wantCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, "find") + "?name=" + tt.s
			rsp, err := tc.do(tt.user, get, path, nil, nil)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if tt.want != nil {
				got, err := parseResponse[[]hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{})); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}
