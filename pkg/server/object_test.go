// SPDX-License-Identifier: MIT

package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"testing"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/compare"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestServer_Object(t *testing.T) {
	srv := newTestServer(".test_obj", t, user1, user2, user3)

	tc := newTestClient(srv, t, adminUser, user1, user2, user3)

	create(tc, t,
		tests.Pool(pool1, tests.UserID(user1ID), tests.Perms(everyone, write), tests.Labels("L1", "K1")),
		tests.Pool(pool2, tests.UserID(user1ID), tests.Perms(everyone, read)),
		tests.Pool(pool3, tests.UserID(user1ID), tests.Perms(user2, write, user3, read)),
		tests.Pool(pool4, tests.UserID(user2ID), tests.Linked(user1ID, pool3)),
	)

	tests := []struct {
		obj      any
		wantObj  *hos.Object
		header   map[string]string
		name     string
		user     string
		method   string
		path     string
		wantObjs []hos.Object
		wantCode int
	}{
		// create test.jpg in pool1(*:w) with user1 client
		{
			name:     "crt1",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
		},
		// create test.jpg again in pool1(*:w) with user1 client, already exists
		{
			name:     "crt2",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileJPG.Copy(),
			wantCode: 409,
		},
		// get test.jpg in pool1(*:w) with user2 client
		{
			name:     "get1",
			user:     user1,
			method:   head,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			wantCode: 204,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
		},
		// get test.jpg in pool1(*:w) with anon user client
		{
			name:     "get2",
			user:     anonUser,
			method:   head,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			wantCode: 204,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
		},
		// create test.dat in pool1(*:w) with user2 client
		{
			name:     "crt3",
			user:     user2,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileAVI.Copy(tests.Labels("X", "Z")),
			wantCode: 201,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.Labels("X", "Z"),
				tests.UserID(user2ID),
			),
		},
		// edit test.dat in pool1(*:w) with user1 client, all and remove labels
		// also put Size ReplicaCount, UserID which they should not have any effect
		{
			name:   "edit1",
			user:   user1,
			method: post,
			path:   tests.OPath(user1ID, pool1, tests.AVI),
			obj: tests.FileAVI.Copy(
				tests.Labels("L2", "K2", "!X", ""),
				tests.Size(1234),
				tests.RepCount(3),
				tests.UserID(user1ID),
			),
			wantCode: 204,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.Labels("L2", "K2"),
				tests.UserID(user2ID),
			),
		},
		// get test.dat in pool1(*:w) with anon user client
		{
			name:     "get3",
			user:     anonUser,
			method:   head,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			wantCode: 204,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.Labels("L2", "K2"),
				tests.UserID(user2ID),
			),
		},
		// list pool1(*:w) with anon user client
		{
			name:     "list1",
			user:     anonUser,
			method:   get,
			path:     tests.PID(user1ID, pool1),
			wantCode: 200,
			wantObjs: []hos.Object{
				*tests.FileAVI.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.Labels("L2", "K2"),
					tests.UserID(user2ID),
				),
				*tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
			},
		},
		// delete test.dat in pool1(*:w) with anon user client, not allowed
		{
			name:     "del1",
			user:     anonUser,
			method:   del,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			wantCode: 403,
		},
		// delete test.dat in pool1(*:w) with anon user client, test.dat created by user2 but user1 can also delete it
		{
			name:     "del2",
			user:     user1,
			method:   del,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			wantCode: 204,
		},
		// create test.dat in pool1(*:w) with user2 client
		{
			name:     "crt4",
			user:     user2,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileAVI.Copy(),
			wantCode: 201,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.UserID(user2ID),
			),
		},
		// edit test.dat in pool1(*:w) with user1 client, also put Hash, CreateTime, ContentTye
		{
			name:   "edit2",
			user:   user1,
			method: post,
			path:   tests.OPath(user1ID, pool1, tests.AVI),
			obj: tests.FileAVI.Copy(
				tests.Labels("L1", "K8"),
				tests.Hash("21"),
				tests.CreatedAt(time.Now()),
				tests.ContentType("text/plain"),
			),
			wantCode: 204,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.UserID(user2ID),
				tests.Labels("L1", "K8"),
				tests.CreatedAt(time.Now()),
				tests.ContentType("text/plain"),
			),
		},
		// delete test.dat in pool1(*:w) with user3 user client, user3 write perm on pool1, can delete
		{
			name:     "del3",
			user:     user3,
			method:   del,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			wantCode: 204,
		},
		// create test.jpg in pool2(*:r) with user1 client
		{
			name:     "crt5",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool2),
			obj:      tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool2)),
		},
		// get test.jpg in pool2(*:r) with anon user client
		{
			name:     "get4",
			user:     anonUser,
			method:   head,
			path:     tests.OPath(user1ID, pool2, tests.JPG),
			wantCode: 204,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool2)),
		},
		// delete test.jpg in pool2(*:r) with user3 user client, user3 has only read perm on pool2
		{
			name:     "del4",
			user:     user3,
			method:   del,
			path:     tests.OPath(user1ID, pool2, tests.JPG),
			wantCode: 403,
		},
		// list pool2(*:r) with anon user client
		{
			name:     "list2",
			user:     anonUser,
			method:   get,
			path:     tests.PID(user1ID, pool2),
			wantCode: 200,
			wantObjs: []hos.Object{*tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool2))},
		},
		// create test.dat in pool3(user2:w, user3:r) with user1 client
		{
			name:     "crt6",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool3),
			obj:      tests.FileAVI.Copy(),
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3)),
		},
		// get test.dat in pool3(user2:w, user3:r) with anon user client
		{
			name:     "get5",
			user:     anonUser,
			method:   head,
			path:     tests.OPath(user1ID, pool3, tests.AVI),
			wantCode: 403,
		},
		// delete test.dat in pool3(user2:w, user3:r) with user3 user client, user3 has read perm on pool3
		{
			name:     "del5",
			user:     user3,
			method:   del,
			path:     tests.OPath(user1ID, pool3, tests.AVI),
			wantCode: 403,
		},
		// delete test.dat in pool3(user2:w, user3:r) with user3 user client, user3 has read perm on pool3
		{
			name:     "del6",
			user:     user2,
			method:   del,
			path:     tests.OPath(user1ID, pool3, tests.AVI),
			wantCode: 204,
		},
		// create test.dat in pool3(user2:w, user3:r) with user2 client
		{
			name:     "crt7",
			user:     user2,
			method:   put,
			path:     tests.PID(user1ID, pool3),
			obj:      tests.FileAVI.Copy(),
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// delete test.dat in pool3(user2:w, user3:r) with user3 user client, user3 has read perm on pool3
		{
			name:     "del7",
			user:     user3,
			method:   del,
			path:     tests.OPath(user1ID, pool3, tests.AVI),
			wantCode: 403,
		},
		// get test.dat in pool3(user2:w, user3:r) with user1 client, user1 is owner of pool3
		{
			name:     "get6",
			user:     user1,
			method:   head,
			path:     tests.OPath(user1ID, pool3, tests.AVI),
			wantCode: 204,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// list pool3(user2:w, user3:r) with user1 client
		{
			name:     "list3",
			user:     user1,
			method:   get,
			path:     tests.PID(user1ID, pool3),
			wantCode: 200,
			wantObjs: []hos.Object{
				*tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
			},
		},
		// list pool3(user2:w, user3:r) with admin user client
		{
			name:     "list4",
			user:     adminUser,
			method:   get,
			path:     tests.PID(user1ID, pool3),
			wantCode: 401,
		},
		// delete test.dat in pool3(user2:w, user3:r) with user1 user client
		{
			name:     "del8",
			user:     user1,
			method:   del,
			path:     tests.OPath(user1ID, pool3, tests.AVI),
			wantCode: 204,
		},
		// create test.dat in pool3(user2:w, user3:r) with user1 client
		{
			name:     "crt8",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool3),
			obj:      tests.FileAVI.Copy(),
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3)),
		},
		// create test.dat in pool4 link to pool3(user2:w, user3:r) with user2 client
		{
			name:     "crt9",
			user:     user2,
			method:   put,
			path:     tests.PID(user2ID, pool4),
			obj:      tests.FileAVI.Copy(),
			wantCode: 409,
		},
		// create test.jpg in pool4 link to pool3(user2:w, user3:r) with user2 client
		{
			name:     "crt10",
			user:     user2,
			method:   put,
			path:     tests.PID(user2ID, pool4),
			obj:      tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// create test.csv in pool4 link to pool3(user2:w, user3:r) with user2 client, without size it will fail
		{
			name:     "crt11",
			user:     user2,
			method:   put,
			path:     tests.PID(user2ID, pool4),
			obj:      tests.FileCSV.Copy(tests.Size(0)),
			wantCode: 411,
		},
		// get test.dat from pool4 link to pool3(user2:w, user3:r) with user2 client, user1 is owner of pool4
		{
			name:     "get7",
			user:     user2,
			method:   head,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.AVI)),
			wantCode: 204,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3)),
		},
		// get test.jpg from pool4 link to pool3(user2:w, user3:r) with user2 client, user1 is owner of pool4
		{
			name:     "get8",
			user:     user2,
			method:   head,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.JPG)),
			wantCode: 204,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// get test.dat from pool4 link to pool3(user2:w, user3:r) with user1 client, not allowed
		{
			name:     "get9",
			user:     user1,
			method:   head,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.AVI)),
			wantCode: 444,
		},
		// get test.jpg from pool4 link to pool3(user2:w, user3:r) with user1 client, not allowed
		{
			name:     "get10",
			user:     user1,
			method:   head,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.JPG)),
			wantCode: 444,
		},
		// list pool4 link to pool3(user2:w, user3:r) with user1 client, not allowed
		{
			name:     "list5",
			user:     user1,
			method:   get,
			path:     tests.PID(user2ID, pool4),
			wantCode: 444,
		},
		// list pool4 link to pool3(user2:w, user3:r) with user3 client, not allowed
		{
			name:     "list6",
			user:     user3,
			method:   get,
			path:     tests.PID(user2ID, pool4),
			wantCode: 444,
		},
		// list pool4 link to pool3(user2:w, user3:r) with user2 client, not allowed
		{
			name:     "list7",
			user:     user2,
			method:   get,
			path:     tests.PID(user2ID, pool4),
			wantCode: 200,
			wantObjs: []hos.Object{
				*tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3)),
				*tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
			},
		},
		// download test.jpg from pool4 link to pool3(user2:w, user3:r) with admin user
		{
			name:     "down1",
			user:     adminUser,
			method:   get,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.JPG)),
			wantCode: 401,
		},
		// download test.jpg from pool4 link to pool3(user2:w, user3:r) with admin user
		{
			name:     "down2",
			user:     adminUser,
			method:   get,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.JPG)),
			wantCode: 200,
			header:   map[string]string{header.OnBehalf: user2},
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// delete test.dat from pool4 link to pool3(user2:w, user3:r) with user1 client, not allowed
		{
			name:     "del9",
			user:     user1,
			method:   del,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.AVI)),
			wantCode: 444,
		},
		// delete test.dat from pool4 link to pool3(user2:w, user3:r) with admin client, not allowed
		{
			name:     "del10",
			user:     adminUser,
			method:   del,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.AVI)),
			wantCode: 401,
		},
		// delete test.jpg from pool4 link to pool3(user2:w, user3:r) with user2 client, user2 is owner
		{
			name:     "del11",
			user:     user2,
			method:   del,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.JPG)),
			wantCode: 204,
		},
		// delete test.dat from pool4 link to pool3(user2:w, user3:r) with admin client on behalf of user2,
		// test.dat is created by user1 but user3 has write perm to pool3, which pool4 is link to
		{
			name:     "del12",
			user:     adminUser,
			method:   del,
			path:     tests.OLinkPath(user2ID, pool4, tests.OID(user1ID, pool3, tests.AVI)),
			wantCode: 204,
			header:   map[string]string{header.OnBehalf: user2},
		},
		// list pool4 link to pool3(user2:w, user3:r) with admin client on behalf of user2
		{
			name:     "list8",
			user:     adminUser,
			method:   get,
			path:     tests.PID(user2ID, pool4),
			wantCode: 200,
			header:   map[string]string{header.OnBehalf: user2},
			wantObjs: []hos.Object{},
		},
		// create /test.jpg in pool1(*:w) with user1 client, invalid object name
		{
			name:     "crt12",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileJPG.Copy(tests.Name("/test.jpg")),
			wantCode: 400,
		},
		// create test.jpg in pool1(*:w) with user1 client, content type is required
		{
			name:     "crt13",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileJPG.Copy(tests.ContentType("")),
			wantCode: 415,
		},
		// create test.csv in pool1(*:w) with user1 client, without size
		{
			name:     "crt14",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			obj:      tests.FileCSV.Copy(tests.Size(0)),
			wantCode: 201,
			header:   map[string]string{header.SizeUnknown: "yes"},
			wantObj:  tests.FileCSV.Obj(tests.UserPoolID(user1ID, pool1)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.obj, tt.header)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if tt.wantObj != nil {
				got, err := parseResponse[hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(&got, tt.wantObj, cmpopts.IgnoreUnexported(hos.Object{}),
					cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}

			if tt.wantObjs != nil {
				got, err := parseResponse[[]hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(got, tt.wantObjs, cmpopts.IgnoreUnexported(hos.Object{}),
					cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestServer_ObjectContent(t *testing.T) {
	srv := newTestServer(".test_content", t, user1)

	tc := newTestClient(srv, t, adminUser, user1)

	create(tc, t,
		tests.Pool(pool1, tests.UserID(user1ID)),
		tests.Pool(pool2, tests.UserID(user1ID), tests.Encrypted()),
		map[string]string{
			"user":                  user1,
			header.EncryptionNewKey: tests.Key1,
		},
		map[string]string{
			"user":                  user1,
			header.EncryptionNewKey: tests.Key2,
			header.EncryptionKey:    tests.Key1,
		},
	)

	tests := []struct {
		header   map[string]string
		file     any
		name     string
		user     string
		method   string
		path     string
		hash     string
		length   int64
		wantCode int
	}{
		// create test.jpg
		{
			name:     "ct1",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			file:     tests.FileJPG.Copy(),
			wantCode: 201,
		},
		// get all test.jpg
		{
			name:     "get1",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			length:   1024,
			hash:     tests.FileJPG.Hash,
			wantCode: 200,
		},
		// create test.avi
		{
			name:     "ct2",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			file:     tests.FileAVI.Copy(),
			wantCode: 201,
		},
		// get all test.avi
		{
			name:     "get2",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			length:   2048,
			hash:     tests.FileAVI.Hash,
			wantCode: 200,
		},
		// get partial test.avi
		{
			name:     "get3",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			length:   48,
			header:   map[string]string{"Range": "bytes=0-47"},
			hash:     "68ce02191fd5a52021ba0db89831f178fef8617a437dbb87daa018ef21f7b0db",
			wantCode: 206,
		},
		// get partial test.avi
		{
			name:     "get4",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool1, tests.AVI),
			length:   1024,
			header:   map[string]string{"Range": "bytes=1024-2050"},
			hash:     "fe433059c7f5e1920b6e6562ea1aa0bb32ae19c93ed756e617782edc7e61fa57",
			wantCode: 206,
		},
		// create test.avi encrypted without key
		{
			name:     "ct3",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool2),
			file:     tests.FileAVI.Copy(),
			wantCode: 400,
		},
		// create test.avi encrypted
		{
			name:     "ct4",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool2),
			header:   map[string]string{header.EncryptionKey: tests.Key1},
			file:     tests.FileAVI.Copy(),
			wantCode: 201,
		},
		// get all encrypted test.avi without the key
		{
			name:     "get5",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool2, tests.AVI),
			wantCode: 400,
		},
		// get all encrypted test.avi
		{
			name:     "get6",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool2, tests.AVI),
			length:   2048,
			header:   map[string]string{header.EncryptionKey: tests.Key1},
			hash:     tests.FileAVI.Hash,
			wantCode: 200,
		},
		// get partial test.jpg
		{
			name:     "get7",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool2, tests.AVI),
			length:   48,
			header:   map[string]string{header.EncryptionKey: tests.Key1, "Range": "bytes=0-47"},
			hash:     "68ce02191fd5a52021ba0db89831f178fef8617a437dbb87daa018ef21f7b0db",
			wantCode: 206,
		},
		// get partial encrypted test.avi with key2
		{
			name:     "get8",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool2, tests.AVI),
			length:   1024,
			header:   map[string]string{header.EncryptionKey: tests.Key2, "Range": "bytes=1024-2050"},
			hash:     "fe433059c7f5e1920b6e6562ea1aa0bb32ae19c93ed756e617782edc7e61fa57",
			wantCode: 206,
		},
		// get encrypted test.avi, with wrong key
		{
			name:     "get9",
			user:     user1,
			method:   get,
			path:     tests.OPath(user1ID, pool2, tests.AVI),
			header:   map[string]string{header.EncryptionKey: tests.Key3},
			wantCode: 417,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.file, tt.header)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				msg, _ := io.ReadAll(rsp.Body)
				t.Errorf("Expected %s(%d), got %s(%d), error %s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode,
					string(msg),
				)
			}

			if rsp.StatusCode >= 400 || rsp.StatusCode == 201 {
				return
			}
			defer rsp.Body.Close()

			if rsp.ContentLength != tt.length {
				t.Errorf("Length expected %d, got %d", tt.length, rsp.ContentLength)
			}

			hw := sha256.New()
			if _, err := io.Copy(hw, rsp.Body); err != nil {
				t.Errorf("copy error: %s", err)
			}

			if hash := fmt.Sprintf("%x", hw.Sum(nil)); hash != tt.hash {
				t.Errorf("hash expected %s, got %s", tt.hash, hash)
			}
		})
	}
}

func TestServer_ObjectListFilter(t *testing.T) {
	srv := newTestServer(".test_obj", t, user1)

	tc := newTestClient(srv, t, adminUser, user1)

	files := []*tests.File{
		tests.FileCSV.Copy(tests.Name("abc"), tests.UserPoolID(user1ID, pool1), tests.Labels("num", "even")),
		tests.FileCSV.Copy(tests.Name("abd"), tests.UserPoolID(user1ID, pool1), tests.Labels("num", "odd")),
		tests.FileCSV.Copy(tests.Name("abf"), tests.UserPoolID(user1ID, pool1), tests.Labels("num", "even")),
		tests.FileCSV.Copy(tests.Name("acd"), tests.UserPoolID(user1ID, pool1), tests.Labels("num", "odd")),
		tests.FileCSV.Copy(tests.Name("acf"), tests.UserPoolID(user1ID, pool1), tests.Labels("num", "even")),
	}

	objs := []hos.Object{}
	aa := []any{tests.Pool(pool1, tests.UserID(user1ID))}
	for _, f := range files {
		aa = append(aa, f)
		objs = append(objs, *f.Obj(tests.UserPoolID(user1ID, pool1)))
	}
	create(tc, t, aa...)

	testCases := []struct {
		opts     *filter.Headers
		wantOpts *filter.Headers
		name     string
		user     string
		wantObjs []hos.Object
		wantCode int
	}{
		// get objs starts with a
		{
			name:     "get1",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "a",
			},
			wantObjs: objs,
		},
		// get objs starts with ab
		{
			name:     "get2",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ab",
			},
			wantObjs: objs[:3],
		},
		// get objs starts with ac
		{
			name:     "get3",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ac",
			},
			wantObjs: objs[3:],
		},
		// get objs starts with a, range 2
		{
			name:     "get4",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "a",
				Range:      []int{1, 2},
			},
			wantObjs: objs[1:3],
		},
		// get objs starts with ac, range 5
		{
			name:     "get5",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ac",
				Range:      []int{0, 5},
			},
			wantOpts: &filter.Headers{
				NamePrefix: "ac",
				Range:      []int{0, 2},
			},
			wantObjs: objs[3:],
		},
		// get objs starts with ab, label num even
		{
			name:     "get6",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ab",
				Labels: []filter.Label{
					{Key: "num", Value: "even", Equal: true},
				},
			},
			wantObjs: []hos.Object{objs[0], objs[2]},
		},
		// get objs starts with ab, label num even and not equal to even
		{
			name:     "get7",
			user:     user1,
			wantCode: 200,
			opts: &filter.Headers{
				NamePrefix: "ab",
				Labels: []filter.Label{
					{Key: "num", Value: "even", Equal: false},
					{Key: "num", Value: "even", Equal: true},
				},
			},
			wantObjs: []hos.Object{},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tests.PID(user1ID, pool1))
			rsp, err := tc.do(tt.user, get, path, nil, header.Serialize(tt.opts))
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d)",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode)
			}

			if tt.wantObjs != nil {
				got, err := parseResponse[[]hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				slices.SortFunc(got, compare.Object)

				if diff := cmp.Diff(got, tt.wantObjs, cmpopts.IgnoreUnexported(hos.Object{}),
					cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
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

func TestServer_ObjectMove(t *testing.T) {
	srv := newTestServer(".test_obj", t, user1, user2)

	tc := newTestClient(srv, t, adminUser, user1, user2)

	create(tc, t,
		tests.Pool(pool1, tests.UserID(user1ID), tests.Perms(everyone, write)),
		tests.Pool(pool2, tests.UserID(user1ID), tests.Perms(everyone, read), tests.Encrypted()),
		tests.Pool(pool3, tests.UserID(user1ID), tests.Perms(user2, write)),
		tests.Pool(pool4, tests.UserID(user1ID), tests.Perms(everyone, read)),
		tests.Pool(pool5, tests.UserID(user1ID), tests.Encrypted()),
		tests.Pool(pool6, tests.UserID(user2ID), tests.Linked(user1ID, pool1)),
		tests.Pool(pool7, tests.UserID(user2ID), tests.Linked(user1ID, pool3)),
		tests.Pool(pool8, tests.UserID(user2ID), tests.Linked(user1ID, pool4)),
		tests.Pool(pool9, tests.UserID(user2ID)),
		map[string]string{"user": user1, header.EncryptionNewKey: tests.Key1},
	)

	tests := []struct {
		wantObj  *hos.Object
		file     any
		header   map[string]string
		name     string
		user     string
		method   string
		path     string
		wantCode int
	}{
		// create test.jpg in pool1(*:w) with user1 client
		{
			name:     "crt1",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			file:     tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
		},
		// move test.jpg from pool1(*:w) to pool2(*:r) with user1 client
		{
			name:     "move1",
			user:     user1,
			method:   patch,
			path:     path.Join(tests.PID(user1ID, pool1), tests.OID(user1ID, pool1, tests.JPG), tests.PID(user1ID, pool2)),
			wantCode: 433,
		},
		// move test.jpg from pool6(*:w) to pool7(user2:w) with user2 client
		{
			name:     "move2",
			user:     user2,
			method:   patch,
			path:     path.Join(tests.PID(user2ID, pool6), tests.OID(user1ID, pool1, tests.JPG), tests.PID(user2ID, pool7)),
			wantCode: 204,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3)),
		},
		// delete test.dat in pool1(*:w) with anon user client, not allowed
		{
			name:     "del1",
			user:     user1,
			method:   del,
			path:     tests.OPath(user1ID, pool1, tests.JPG),
			wantCode: 404,
		},
		// move test.jpg from pool7(*:w) to pool8(*:r) with user2 client
		{
			name:     "move3",
			user:     user2,
			method:   patch,
			path:     path.Join(tests.PID(user2ID, pool7), tests.OID(user1ID, pool1, tests.JPG), tests.PID(user2ID, pool8)),
			wantCode: 403,
		},
		// create test.jpg in pool4(*:r) with user1 client
		{
			name:     "crt2",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool4),
			file:     tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool4)),
		},
		// move test.jpg from pool8(*:r) to pool7(*:w) with user2 client
		{
			name:     "move4",
			user:     user2,
			method:   patch,
			path:     path.Join(tests.PID(user2ID, pool8), tests.OID(user1ID, pool4, tests.JPG), tests.PID(user2ID, pool7)),
			wantCode: 403,
		},
		// create test.jpg encrypted in pool2 with user1 client
		{
			name:     "crt3",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool2),
			file:     tests.FileJPG.Copy(),
			wantCode: 201,
			header:   map[string]string{header.EncryptionKey: tests.Key1},
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool2), tests.Encrypted()),
		},
		// move test.jpg from pool2 to pool5 with user1 client with name change
		{
			name:     "move5",
			user:     user1,
			method:   patch,
			path:     path.Join(tests.PID(user1ID, pool2), tests.OID(user1ID, pool2, tests.JPG), tests.PID(user1ID, pool5)),
			wantCode: 204,
			header:   map[string]string{header.NewObjectName: "test1.jpg"},
			wantObj: tests.FileJPG.Obj(
				tests.UserPoolID(user1ID, pool5),
				tests.Name("test1.jpg"),
				tests.Encrypted(),
			),
		},
		// delete pool2 with user1
		{
			name:     "del2",
			user:     user1,
			method:   del,
			path:     tests.PID(user1ID, pool2),
			wantCode: 204,
		},
		// move test.jpg from pool7(user2:w) to pool6(*:w) with user2 client with name change
		{
			name:     "move6",
			user:     user2,
			method:   patch,
			path:     path.Join(tests.PID(user2ID, pool7), tests.OID(user1ID, pool3, tests.JPG), tests.PID(user2ID, pool6)),
			wantCode: 204,
			header:   map[string]string{header.NewObjectName: "test2.jpg"},
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1), tests.Name("test2.jpg")),
		},
		// move test.jpg from pool6(*:w) to pool9(*:w) with user2
		{
			name:     "move7",
			user:     user2,
			method:   patch,
			path:     path.Join(tests.PID(user2ID, pool6), tests.OID(user1ID, pool1, "test2.jpg"), tests.PID(user2ID, pool9)),
			wantCode: 444,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.file, tt.header)
			if err != nil {
				t.Fatal(err)
			}

			mb, _ := io.ReadAll(rsp.Body)
			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d), error=%s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode,
					string(mb),
				)
			}

			if tt.wantObj != nil {
				got, err := parseResponse[hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				if diff := cmp.Diff(&got, tt.wantObj, cmpopts.IgnoreUnexported(hos.Object{}),
					cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestServer_ObjectCopyRemote(t *testing.T) {
	srv1 := newTestServer(".test1_obj", t, user1, user2)
	srv2 := newTestServer(".test2_obj", t, user1, user2)

	tc1 := newTestClient(srv1, t, adminUser, user1, user2)
	tc2 := newTestClient(srv2, t, adminUser, user1, user2)

	pools := []any{
		tests.Pool(pool1, tests.UserID(user1ID), tests.Perms(everyone, write)),
		tests.Pool(pool2, tests.UserID(user1ID), tests.Perms(everyone, read), tests.Encrypted()),
		tests.Pool(pool3, tests.UserID(user1ID), tests.Perms(user2, write)),
		tests.Pool(pool4, tests.UserID(user1ID), tests.Encrypted()),
		tests.Pool(pool5, tests.UserID(user2ID), tests.Linked(user1ID, pool3)),
		tests.Pool(pool6, tests.UserID(user2ID)),
	}

	create(tc1, t,
		append(pools, map[string]string{"user": user1, header.EncryptionNewKey: tests.Key1})...,
	)
	create(tc2, t,
		append(pools, map[string]string{"user": user1, header.EncryptionNewKey: tests.Key2})...,
	)

	tests := []struct {
		C        *clientT
		wantObj  *hos.Object
		header   map[string]string
		data     any
		name     string
		user     string
		method   string
		path     string
		wantCode int
	}{
		// create test.jpg in pool1(*:w) with user1 client
		{
			name:     "crt1",
			user:     user1,
			C:        tc1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			data:     tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
		},
		// copy test.jpg from srv1 pool1(*:w) to srv2 pool3 with user2 client
		{
			name:   "copy1",
			user:   user2,
			C:      tc1,
			method: patch,
			data:   srv2.caCert,
			path:   tests.OPath(user1ID, pool1, tests.JPG),
			header: map[string]string{
				header.DestServer: srv2.srv.Addr,
				header.DestToken:  tc2.tokens[user2],
				header.DestPool:   pool5,
			},
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// create test.avi in pool2(*:r) with user1 client
		{
			name:   "crt2",
			user:   user1,
			C:      tc1,
			method: put,
			path:   tests.PID(user1ID, pool2),
			data:   tests.FileAVI.Copy(),
			header: map[string]string{
				header.EncryptionKey: tests.Key1,
			},
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool2), tests.Encrypted()),
		},
		// copy test.avi from srv1 pool2(*:r) to srv2 pool5 with user1 client
		// decrypt and re-encrypt
		{
			name:   "copy2",
			user:   user1,
			C:      tc1,
			method: patch,
			data:   srv2.caCert,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestServer:       srv2.srv.Addr,
				header.DestToken:        tc2.tokens[user1],
				header.DestPool:         pool4,
				header.EncryptionKey:    tests.Key1,
				header.EncryptionNewKey: tests.Key2,
			},
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool4), tests.Encrypted()),
		},
		// copy test.avi from srv1 pool2(*:r) to srv2 pool9 with user2 client
		// copy encrypted file as it is
		{
			name:   "copy3",
			user:   user2,
			C:      tc1,
			method: patch,
			data:   srv2.caCert,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestServer: srv2.srv.Addr,
				header.DestToken:  tc2.tokens[user2],
				header.DestPool:   pool6,
			},
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user2ID, pool6), tests.Size(2080)),
		},
		// copy test.avi from srv1 pool2(*:r) to srv1 pool3 with user1
		{
			name:   "copy4",
			user:   user1,
			C:      tc1,
			method: patch,
			data:   srv1.caCert,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestServer: srv1.srv.Addr,
				header.DestToken:  tc1.tokens[user1],
				header.DestPool:   pool4,
			},
			wantCode: 444,
		},
		// copy test.avi from srv1 pool2(*:r) to srv2 pool3 with user1
		// same name will throw an error
		{
			name:   "copy5",
			user:   user1,
			C:      tc1,
			method: patch,
			data:   srv2.caCert,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestServer:       srv2.srv.Addr,
				header.DestToken:        tc2.tokens[user1],
				header.DestPool:         pool4,
				header.EncryptionNewKey: tests.Key2,
			},
			wantCode: 409,
		},
		// copy test.dat from srv1 pool2(*:r) to srv2 pool5 with user1 and a new name
		{
			name:   "copy6",
			user:   user1,
			C:      tc1,
			method: patch,
			data:   srv2.caCert,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestServer:       srv2.srv.Addr,
				header.DestToken:        tc2.tokens[user1],
				header.DestPool:         pool4,
				header.NewObjectName:    "test1.avi",
				header.EncryptionNewKey: tests.Key2,
			},
			wantCode: 201,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool4),
				tests.Name("test1.avi"),
				tests.Size(2080),
				tests.Encrypted(),
				tests.Hash(""),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tt.C.do(tt.user, tt.method, path, tt.data, tt.header)
			if err != nil {
				t.Fatal(err)
			}

			mb, _ := io.ReadAll(rsp.Body)
			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d), error=%s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode,
					string(mb),
				)
			}

			if tt.wantObj != nil {
				got, err := parseResponse[hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				if tt.wantObj != nil && tt.wantObj.Hash == "" {
					got.Hash = ""
				}

				if diff := cmp.Diff(&got, tt.wantObj, cmpopts.IgnoreUnexported(hos.Object{}),
					cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestServer_ObjectCopyLocal(t *testing.T) {
	srv := newTestServer(".test_copy_local", t, user1, user2)

	tc := newTestClient(srv, t, adminUser, user1, user2)

	create(tc, t,
		tests.Pool(pool1, tests.UserID(user1ID), tests.Perms(everyone, write)),
		tests.Pool(pool2, tests.UserID(user1ID), tests.Perms(everyone, read), tests.Encrypted()),
		tests.Pool(pool3, tests.UserID(user1ID), tests.Perms(user2, write)),
		tests.Pool(pool4, tests.UserID(user1ID), tests.Encrypted()),
		tests.Pool(pool5, tests.UserID(user2ID), tests.Linked(user1ID, pool3)),
		tests.Pool(pool6, tests.UserID(user2ID)),
		tests.Pool(pool7, tests.UserID(user2ID), tests.Linked(user1ID, pool4)),
		map[string]string{"user": user1, header.EncryptionNewKey: tests.Key1},
	)

	tests := []struct {
		wantObj  *hos.Object
		header   map[string]string
		data     any
		name     string
		user     string
		method   string
		path     string
		wantCode int
	}{
		// create test.jpg in pool1(*:w) with user1 client
		{
			name:     "crt1",
			user:     user1,
			method:   put,
			path:     tests.PID(user1ID, pool1),
			data:     tests.FileJPG.Copy(),
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool1)),
		},
		// copy test.jpg from srv1 pool1(*:w) to srv2 pool3 with user2 client
		{
			name:   "copy1",
			user:   user2,
			method: patch,
			path:   tests.OPath(user1ID, pool1, tests.JPG),
			header: map[string]string{
				header.DestPool: pool5,
			},
			wantCode: 201,
			wantObj:  tests.FileJPG.Obj(tests.UserPoolID(user1ID, pool3), tests.UserID(user2ID)),
		},
		// create test.avi in pool2(*:r) with user1 client
		{
			name:   "crt2",
			user:   user1,
			method: put,
			path:   tests.PID(user1ID, pool2),
			data:   tests.FileAVI.Copy(),
			header: map[string]string{
				header.EncryptionKey: tests.Key1,
			},
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool2), tests.Encrypted()),
		},
		// copy test.avi from srv1 pool2(*:r) to srv2 pool5 with user1 client
		// decrypt and re-encrypt
		{
			name:   "copy2",
			user:   user1,
			method: patch,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestPool:      pool4,
				header.EncryptionKey: tests.Key1,
			},
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool4), tests.Encrypted()),
		},
		// copy test.avi from srv1 pool2(*:r) to pool6 without enc key
		{
			name:   "copy3",
			user:   user2,
			method: patch,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestPool: pool6,
			},
			wantCode: 400,
		},
		// copy test.avi from pool2(*:r) to pool3
		{
			name:   "copy4",
			user:   user1,
			method: patch,
			data:   srv.caCert,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestPool:      pool3,
				header.EncryptionKey: tests.Key1,
			},
			wantCode: 201,
			wantObj:  tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool3)),
		},
		// copy test.avi pool2(*:r) to pool2 with same name
		// same name will throw an error
		{
			name:   "copy5",
			user:   user1,
			method: patch,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestPool: pool2,
			},
			wantCode: 409,
		},
		// copy test.avi  pool2(*:r) to pool2
		{
			name:   "copy6",
			user:   user1,
			method: patch,
			path:   tests.OPath(user1ID, pool2, tests.AVI),
			header: map[string]string{
				header.DestPool:         pool2,
				header.NewObjectName:    "test1.avi",
				header.EncryptionNewKey: tests.Key1,
			},
			wantCode: 201,
			wantObj: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool2),
				tests.Name("test1.avi"),
				tests.Encrypted(),
				tests.Hash(""),
			),
		},
		// copy test.jpg  pool1(*:w) to pool4 witout enc key
		{
			name:   "copy7",
			user:   user1,
			method: patch,
			path:   tests.OPath(user1ID, pool1, tests.JPG),
			header: map[string]string{
				header.DestPool: pool4,
			},
			wantCode: 400,
		},
		// copy test.jpg  pool1(*:w) to pool3 with user2
		{
			name:   "copy8",
			user:   user2,
			method: patch,
			path:   tests.OPath(user1ID, pool1, tests.JPG),
			header: map[string]string{
				header.DestPool: pool7,
			},
			wantCode: 403,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.APIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, tt.data, tt.header)
			if err != nil {
				t.Fatal(err)
			}

			mb, _ := io.ReadAll(rsp.Body)
			if rsp.StatusCode != tt.wantCode {
				t.Errorf("Expected %s(%d), got %s(%d), error=%s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode,
					string(mb),
				)
			}

			if tt.wantObj != nil {
				got, err := parseResponse[hos.Object](rsp)
				if err != nil {
					t.Error(err)
				}

				if tt.wantObj != nil && tt.wantObj.Hash == "" {
					got.Hash = ""
				}

				if diff := cmp.Diff(&got, tt.wantObj, cmpopts.IgnoreUnexported(hos.Object{}),
					cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}
