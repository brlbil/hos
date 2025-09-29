// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/compare"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/id"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestClient_CreateObject(t *testing.T) {
	ts := newTestServer(".create_object", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server3), tests.Pool(pool1, tests.RepCount(3), tests.Perms(user3, write)),
		ts.C(user1, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user2, server1, server3), tests.Pool(pool3, tests.RepCount(3)),
		ts.C(user3, server3), tests.Pool(pool4, tests.Linked(user1ID, pool1)),
		ts.C(user3), tests.Pool(pool5, tests.RepCount(3)),
		ts.C(user1), tests.Pool(pool5, tests.RepCount(3), tests.Encrypted()),
	)

	tests := []struct {
		c       *Client
		file    *tests.File
		want    *hos.Object
		name    string
		wantErr error
		opts    []Options
	}{
		{
			// if destination pool missing on some servers only owner can create the destination pool
			// in this case user3 cannot so it will fail with InsufficientResources
			name:    "destination pool is missing on some servers and it is owned by another user",
			c:       ts.C(user3),
			file:    tests.FileCSV.Copy(tests.PoolID(user3ID, pool4)),
			wantErr: hos.ErrInsufficientResources,
		},
		{
			name: "create test.csv in linked pool on all servers",
			c:    ts.C(user1),
			file: tests.FileCSV.Copy(tests.PoolID(user1ID, pool2)),
			want: tests.FileCSV.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(3),
			),
		},
		{
			name: "create test.jpg on all servers with impersonation",
			c:    ts.C(adminUser),
			file: tests.FileJPG.Copy(tests.PoolID(user1ID, pool1)),
			opts: []Options{OnBehalf(user1)},
			want: tests.FileJPG.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(3),
			),
		},
		{
			name:    "create an object without name",
			c:       ts.C(user3),
			file:    &tests.File{PoolID: adminID},
			wantErr: hos.ErrBadRequest,
		},
		{
			name:    "create an object without content type",
			c:       ts.C(user1),
			file:    tests.FileCSV.Copy(tests.PoolID(user1ID, pool2), tests.ContentType("")),
			wantErr: hos.ErrContentTypeRequired,
		},
		{
			name:    "create an object without size",
			c:       ts.C(user1),
			file:    tests.FileAVI.Copy(tests.PoolID(user1ID, pool2), tests.Size(0)),
			wantErr: hos.ErrSizeRequired,
		},
		{
			name: "create an object without size, with sizeunknown header",
			c:    ts.C(user3),
			file: tests.FileAVI.Copy(tests.PoolID(user3ID, pool5), tests.Size(0)),
			opts: []Options{Headers{header.SizeUnknown: "true"}},
			want: tests.FileAVI.Obj(
				tests.UserPoolID(user3ID, pool5),
				tests.RepCount(3),
			),
		},
		{
			name:    "create object on unauthorized server",
			c:       ts.C(user2),
			file:    tests.FileJPG.Copy(tests.PoolID(user2ID, pool3)),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name: "create object on available servers",
			c:    ts.C(user3, server1, server2, server3, server4),
			file: tests.FileJPG.Copy(tests.PoolID(user3ID, pool5)),
			want: tests.FileJPG.Obj(
				tests.UserPoolID(user3ID, pool5),
				tests.RepCount(3),
			),
		},
		{
			name:    "create object replica count more then available servers",
			c:       ts.C(user2, server1, server3),
			file:    tests.FileJPG.Copy(tests.PoolID(user2ID, pool3)),
			wantErr: hos.ErrInsufficientResources,
		},
		{
			name:    "create an object, not enough disk space",
			c:       ts.C(user1),
			file:    tests.FileAVI.Copy(tests.PoolID(user1ID, pool2), tests.Size(tooLargeSize)),
			wantErr: hos.ErrInsufficientResources,
		},
		{
			name:    "create an object, on encrypted pool without a key",
			c:       ts.C(user1),
			file:    tests.FileAVI.Copy(tests.PoolID(user1ID, pool5)),
			wantErr: hos.ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.CreateObject(context.Background(), tt.file.Obj(), tt.file, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.CreateObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(got, tt.want,
				cmpopts.IgnoreUnexported(hos.Object{}),
				cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt"),
			); diff != "" {
				t.Errorf("Client.CreateObject() %s", diff)
				return
			}

			// Let's check if the object created in case of error, this should not happen
			if tt.wantErr != nil {
				got, err := tt.c.GetObject(context.Background(), tt.file.PoolID, id.Gen(tt.file.PoolID, tt.file.Name),
					IgnoreErrors(hos.ErrNotExist))
				if err == nil {
					t.Errorf("on case of error no object should have been returned object: %v", got)
				}
			}
		})
	}
}

func TestClient_EditObject(t *testing.T) {
	ts := newTestServer(".edit_object", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user2, server1, server3), tests.Pool(pool3, tests.RepCount(1)),
		ts.C(user3, server2, server3), tests.Pool(pool4, tests.Linked(user1ID, pool1)),
	)

	create[tests.File](t, ts.C(user1, server2, server3), tests.FileJPG.Copy(tests.PoolID(user1ID, pool1)))

	tests := []struct {
		c       *Client
		obj     *hos.Object
		want    *hos.Object
		name    string
		wantErr error
		opts    []Options
	}{
		{
			name:    "nil object",
			c:       ts.C(user1),
			wantErr: hos.ErrNotInitialized,
		},
		{
			name: "edit object belong to some one else, full client with impersonation",
			c:    ts.C(adminUser),
			obj: tests.Object(tests.JPG,
				tests.UserPoolID(user3ID, pool4),
				tests.ID(tests.OID(user1ID, pool1, tests.JPG)),
				tests.Labels("X", "Y"),
			),
			opts: []Options{OnBehalf(user3)},
			want: tests.FileJPG.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(2),
				tests.Labels("X", "Y"),
			),
		},
		{
			name: "edit object belong to some one else, with client not authorized on all servers",
			c:    ts.C(user2),
			obj: tests.Object(tests.JPG,
				tests.UserPoolID(user2ID, pool2),
				tests.ID(tests.OID(user1ID, pool1, tests.JPG)),
				tests.Labels("!X", ""),
			),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name: "edit CT of the object from linked pool",
			c:    ts.C(user3),
			obj: tests.Object(tests.JPG,
				tests.UserPoolID(user3ID, pool4),
				tests.ID(tests.OID(user1ID, pool1, tests.JPG)),
				tests.Labels("A", "B"),
			),
			want: tests.FileJPG.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(2),
				tests.Labels("X", "Y", "A", "B"),
			),
		},
		{
			name: "edit a not exist object",
			c:    ts.C(user1),
			obj: tests.Object(tests.AVI,
				tests.UserPoolID(user1ID, pool1),
				tests.Labels("A", "B"),
			),
			wantErr: hos.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.EditObject(context.Background(), tt.obj, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{}),
				cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
				t.Errorf("Client.GetPools() %s", diff)
			}
		})
	}
}

func TestClient_GetObject(t *testing.T) {
	ts := newTestServer(".get_object", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.Pool(pool3, tests.Linked(user1ID, pool1)),
	)

	create[tests.File](t, ts.C(user1, server2, server3), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)))

	tests := []struct {
		c        *Client
		want     *hos.Object
		name     string
		poolID   string
		objectID string
		wantErr  error
		opts     []Options
	}{
		{
			name:     "get object from linked pool, on all servers with impersonation",
			poolID:   tests.PID(user3ID, pool3),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(adminUser),
			opts:     []Options{OnBehalf(user3)},
			want: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(2),
			),
		},
		{
			name:     "get object from linked pool with user that not authz, on all servers",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(user2),
			wantErr:  hos.ErrNotAuthorized,
		},
		{
			name:     "get object from linked pool with user that not authz, on all servers, ignore errors",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(user2),
			opts:     []Options{IgnoreErrors(hos.ErrNotExist, hos.ErrNotAuthorized, hos.ErrNotAllCopiesAvailable)},
			want: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(2),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.GetObject(context.Background(), tt.poolID, tt.objectID, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{}),
				cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
				t.Errorf("Client.GetPools() %s", diff)
			}
		})
	}
}

func TestClient_GetContent(t *testing.T) {
	ts := newTestServer(".get_content", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.Pool(pool3, tests.Linked(user1ID, pool1)),
		ts.C(user1), tests.Pool(pool4, tests.RepCount(3), tests.Encrypted()),
	)

	create[string](t, ts.C(user1), tests.Key1)
	key1 := EncryptionKey(tests.Key(tests.Key1))
	key2 := EncryptionKey(tests.Key(tests.Key2))

	create[tests.File](t,
		ts.C(user1), tests.FileAVI.Copy(tests.PoolID(user1ID, pool4), setEncKey(key1)),
		ts.C(user1, server2, server3), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)),
	)
	wantBuf, _ := os.ReadFile(tests.FileAVI.RelPath())

	tests := []struct {
		c        *Client
		want     *hos.Object
		name     string
		poolID   string
		objectID string
		wantErr  error
		opts     []Options
	}{
		{
			name:     "get object from linked pool, on all servers with impersonation",
			poolID:   tests.PID(user3ID, pool3),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(adminUser),
			opts:     []Options{OnBehalf(user3)},
			want: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(2),
			),
		},
		{
			name:     "get object from linked pool with user that not authz, on all servers",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(user2),
			wantErr:  hos.ErrNotAllCopiesAvailable,
		},
		{
			name:     "get object from linked pool with user that not authz, on all servers, ignore all errors",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(user2),
			opts:     []Options{IgnoreErrors(hos.ErrNotExist, hos.ErrNotAuthorized, hos.ErrNotAllCopiesAvailable)},
			want: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool1),
				tests.RepCount(2),
			),
		},
		{
			name:     "get not exist object",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.CSV),
			c:        ts.C(user2),
			opts:     []Options{IgnoreErrorsExcept()},
			wantErr:  hos.ErrNotExist,
		},
		{
			name:     "get object from encrypted pool",
			poolID:   tests.PID(user1ID, pool4),
			objectID: tests.OID(user1ID, pool4, tests.AVI),
			c:        ts.C(user1),
			opts:     []Options{key1},
			want: tests.FileAVI.Obj(
				tests.UserPoolID(user1ID, pool4),
				tests.RepCount(3),
				tests.Encrypted(),
			),
		},
		{
			name:     "get object from encrypted pool with wrong enc key",
			poolID:   tests.PID(user1ID, pool4),
			objectID: tests.OID(user1ID, pool4, tests.AVI),
			c:        ts.C(user1),
			opts:     []Options{key2},
			wantErr:  hos.ErrDecryption,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.GetContent(context.Background(), tt.poolID, tt.objectID, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{}),
				cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt")); diff != "" {
				t.Errorf("Client.GetPools() %s", diff)
			}

			if tt.wantErr != nil {
				return
			}
			defer got.Close()

			gotBuf, err := io.ReadAll(got)
			if err != nil {
				t.Errorf("Client.GetPool() read data %v", err)
				return
			}

			if diff := cmp.Diff(gotBuf, wantBuf); diff != "" {
				t.Errorf("Client.GetPools() %s", diff)
			}
		})
	}
}

func TestClient_ListObjects(t *testing.T) {
	ts := newTestServer(".list_object", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.Pool(pool3, tests.Linked(user1ID, pool1)),
	)

	create[tests.File](t,
		ts.C(user1, server2, server3), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.FileCSV.Copy(tests.PoolID(user3ID, pool3)),
	)

	tests := []struct {
		c       *Client
		name    string
		poolID  string
		wantErr error
		want    []hos.Object
		opts    []Options
	}{
		{
			name:    "empty pool ID",
			poolID:  "",
			c:       ts.C(user1),
			wantErr: hos.ErrBadRequest,
		},
		{
			name:    "list objects on all servers",
			poolID:  tests.PID(user3ID, pool3),
			c:       ts.C(user3),
			wantErr: hos.ErrNotExist,
		},
		{
			name:   "list objects on all servers, ignore errors with impersonation",
			poolID: tests.PID(user3ID, pool3),
			c:      ts.C(adminUser),
			opts:   []Options{IgnoreErrors(hos.ErrNotExist), OnBehalf(user3)},
			want: []hos.Object{
				*tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool1), tests.RepCount(2)),
				*tests.FileCSV.Obj(tests.UserPoolID(user1ID, pool1), tests.RepCount(2)),
			},
		},
		{
			name:    "list objects on all servers with partial user",
			poolID:  tests.PID(user2ID, pool2),
			c:       ts.C(user2),
			wantErr: hos.ErrNotExist,
		},
		{
			name:   "list objects on all servers with partial user, ignore errors",
			poolID: tests.PID(user2ID, pool2),
			c:      ts.C(user2),
			opts:   []Options{IgnoreErrors(hos.ErrNotExist, hos.ErrNotAuthorized)},
			want: []hos.Object{
				*tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool1), tests.RepCount(2)),
				*tests.FileCSV.Obj(tests.UserPoolID(user1ID, pool1), tests.RepCount(2)),
			},
		},
		{
			name:    "list not exist, ignore errors",
			poolID:  tests.PID(user2ID, pool3),
			c:       ts.C(user2),
			wantErr: hos.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.ListObjects(context.Background(), tt.poolID, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.ListObjects() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{}), cmpopts.IgnoreFields(hos.Object{},
				"UserID", "PoolID", "Hash", "CreatedAt", "ModifiedAt", "Labels")); diff != "" {
				t.Errorf("Client.ListObjects() %s", diff)
			}
		})
	}
}

func TestClient_FilterObjects(t *testing.T) {
	ts := newTestServer(".filter_object", t, createUser(user1))

	create[hos.Pool](t, ts.C(user1), tests.Pool(pool1, tests.RepCount(2)))
	create[tests.File](t,
		ts.C(user1), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1), tests.Name("dir1/dir2/dir3/"+tests.AVI)),
		ts.C(user1), tests.FileCSV.Copy(tests.PoolID(user1ID, pool1), tests.Name("dir1/dir2/"+tests.CSV)),
		ts.C(user1), tests.FileJPG.Copy(tests.PoolID(user1ID, pool1), tests.Name("dir1/"+tests.JPG)),
	)

	pid := tests.PID(user1ID, pool1)
	tests := []struct {
		c       *Client
		name    string
		poolID  string
		wantErr error
		want    []hos.Object
		opts    []Options
	}{
		{
			name:   "filter object on server",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{filter.NamePrefix("dir1/dir2/dir3")},
			want: []hos.Object{
				*tests.FileAVI.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/dir2/dir3/"+tests.AVI),
				),
			},
		},
		{
			name:   "filter object by name",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{FilterByField("ID", "42784ad2", "d5516342")},
			want: []hos.Object{
				*tests.FileAVI.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/dir2/dir3/"+tests.AVI),
				),
				*tests.FileJPG.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/"+tests.JPG),
				),
			},
		},
		{
			name:   "list objects with wrong prefix",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{ObjectDirectoryListing("none")},
			want:   []hos.Object{},
		},
		{
			name:   "list objects with empty prefix",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{ObjectDirectoryListing("")},
			want: []hos.Object{
				*tests.FileAVI.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/"),
					tests.Size(3096),
				),
			},
		},
		{
			name:   "list objects with dir1 prefix",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{ObjectDirectoryListing("dir1")},
			want: []hos.Object{
				*tests.FileAVI.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/dir2/"),
					tests.Size(2072),
				),
				*tests.FileJPG.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/"+tests.JPG),
				),
			},
		},
		{
			name:   "list objects with dir1/test.jpg prefix",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{FilterByField("Name", "dir1/"+tests.JPG+"/", "dir1/"+tests.JPG)},
			want: []hos.Object{
				*tests.FileJPG.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/"+tests.JPG),
				),
			},
		},
		{
			name:   "list objects with dir1/dir2/ prefix",
			poolID: pid,
			c:      ts.C(user1),
			opts:   []Options{ObjectDirectoryListing("dir1/dir2/")},
			want: []hos.Object{
				*tests.FileAVI.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/dir2/dir3/"),
				),
				*tests.FileCSV.Obj(
					tests.UserPoolID(user1ID, pool1),
					tests.RepCount(2),
					tests.Name("dir1/dir2/"+tests.CSV),
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.ListObjects(context.Background(), tt.poolID, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.ListObjects() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			slices.SortFunc(got, compare.Object)

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{}), cmpopts.IgnoreFields(hos.Object{},
				"UserID", "PoolID", "ID", "Hash", "CreatedAt", "ContentType", "ModifiedAt", "Labels")); diff != "" {
				t.Errorf("Client.ListObjects() %s", diff)
			}
		})
	}
}

func TestClient_MoveObject(t *testing.T) {
	ts := newTestServer(".move_object", t, createUser(user1), createUser(user2))

	create[hos.Pool](t,
		ts.C(user1), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2), tests.Pool(pool2, tests.RepCount(2)),
		ts.C(user2), tests.Pool(pool3, tests.Linked(user1ID, pool1)),
		ts.C(user1), tests.Pool(pool4, tests.RepCount(1)),
		ts.C(user1), tests.Pool(pool5, tests.RepCount(2)),
	)

	create[tests.File](t, ts.C(user1), tests.FileCSV.Copy(tests.PoolID(user1ID, pool1)))

	tests := []struct {
		c         *Client
		wantErr   error
		name      string
		poolID    string
		objectID  string
		dstPoolID string
		srcName   string
		newName   string
	}{
		{
			name:      "move from pool1 to pool4",
			poolID:    tests.PID(user1ID, pool1),
			objectID:  tests.OID(user1ID, pool1, tests.CSV),
			dstPoolID: tests.PID(user1ID, pool4),
			c:         ts.C(user1),
			wantErr:   hos.ErrNotEqual,
		},
		{
			name:      "move from pool3 to pool2",
			poolID:    tests.PID(user2ID, pool3),
			objectID:  tests.OID(user1ID, pool1, tests.CSV),
			dstPoolID: tests.PID(user2ID, pool2),
			c:         ts.C(user2),
			wantErr:   hos.ErrNotAllowed,
		},
		{
			name:      "rename object",
			poolID:    tests.PID(user1ID, pool1),
			objectID:  tests.OID(user1ID, pool1, tests.CSV),
			dstPoolID: tests.PID(user1ID, pool1),
			srcName:   tests.CSV,
			newName:   "test1.csv",
			c:         ts.C(user1),
		},
		{
			name:      "move object",
			poolID:    tests.PID(user1ID, pool1),
			objectID:  tests.OID(user1ID, pool1, "test1.csv"),
			dstPoolID: tests.PID(user1ID, pool5),
			srcName:   "test1.csv",
			c:         ts.C(user1),
		},
	}

	confMap, err := ts.C(user1).ServerConfig(context.Background())
	if err != nil {
		t.Fatal("getting server config failed", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := map[string]string{}
			for host, cnf := range confMap {
				fp := filepath.Join(cnf.RootDir, tt.poolID, tt.objectID)
				if _, err := os.Stat(fp); err == nil {
					sources[host] = fp
				}
			}

			err := tt.c.MoveObject(context.Background(), tt.poolID, tt.objectID, tt.dstPoolID, tt.newName)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("MoveObject() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr != nil {
				return
			}

			for host, src := range sources {
				if _, err := os.Stat(src); err == nil {
					t.Errorf("MoveObject expected %s moves, but it still exists", src)
				}
				cnf := confMap[host]
				var movedFilePath string
				if tt.newName != "" {
					movedFilePath = filepath.Join(cnf.RootDir, tt.dstPoolID, id.Gen(tt.dstPoolID, tt.newName))
				} else {
					movedFilePath = filepath.Join(cnf.RootDir, tt.dstPoolID, id.Gen(tt.dstPoolID, tt.srcName))
				}

				if _, err := os.Stat(movedFilePath); err != nil {
					t.Errorf("MoveObject expected file %s to be exists", movedFilePath)
				}
			}
		})
	}
}

func TestClient_DeleteObject(t *testing.T) {
	ts := newTestServer(".delete_object", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.Pool(pool3, tests.Linked(user1ID, pool1)),
		ts.C(user1), tests.Pool(pool4, tests.RepCount(1)),
	)

	create[tests.File](t,
		ts.C(user1, server2, server3), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.FileCSV.Copy(tests.PoolID(user3ID, pool3)),
		ts.C(user1, server2), tests.FileCSV.Copy(tests.PoolID(user1ID, pool4)),
		ts.C(user1, server3), tests.FileCSV.Copy(tests.PoolID(user1ID, pool4)),
	)
	tests := []struct {
		c        *Client
		name     string
		poolID   string
		objectID string
		wantErr  error
		opts     []Options
	}{
		{
			name:     "delete one object with impersonation",
			poolID:   tests.PID(user3ID, pool3),
			objectID: tests.OID(user1ID, pool1, tests.AVI),
			c:        ts.C(adminUser),
			opts:     []Options{OnBehalf(user3)},
		},
		{
			name:     "delete one object on not all servers",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.CSV),
			c:        ts.C(user2, server3),
			wantErr:  hos.ErrNotAllCopiesAvailable,
		},
		{
			name:     "delete one object on all server one of them not authorized but all copies exists",
			poolID:   tests.PID(user2ID, pool2),
			objectID: tests.OID(user1ID, pool1, tests.CSV),
			c:        ts.C(user2),
			wantErr:  hos.ErrNotAuthorized,
		},
		{
			name:     "delete corrupted object",
			poolID:   tests.PID(user1ID, pool4),
			objectID: tests.OID(user1ID, pool4, tests.CSV),
			c:        ts.C(user1),
			wantErr:  hos.ErrCorrupted,
		},
		{
			name:     "delete corrupted object enforce deletion",
			poolID:   tests.PID(user1ID, pool4),
			objectID: tests.OID(user1ID, pool4, tests.CSV),
			c:        ts.C(user1),
			opts:     []Options{IgnoreErrors(hos.ErrCorrupted)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.DeleteObject(context.Background(), tt.poolID, tt.objectID, tt.opts...); !errors.Is(err, tt.wantErr) {
				t.Errorf("DeleteObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr != nil {
				return
			}

			_, err := tt.c.GetObject(context.Background(), tt.poolID, tt.objectID)
			if !errors.Is(err, hos.ErrNotExist) {
				t.Errorf("DeleteObject/GetObject() error = %v, wantErr %v", err, hos.ErrNotExist)
			}
		})
	}
}
