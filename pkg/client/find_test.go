// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestClient_Find(t *testing.T) {
	ts := newTestServer(".find", t, createUser(user1), createUser(user2, server1, server3), createUser(user3))

	create[hos.Pool](t,
		ts.C(user1, server2, server3), tests.Pool(pool1, tests.RepCount(2), tests.Perms(everyone, write)),
		ts.C(user2, server3), tests.Pool(pool2, tests.Linked(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.Pool(pool3, tests.Linked(user1ID, pool1)),
	)

	create[tests.File](t,
		ts.C(user1, server2, server3), tests.FileAVI.Copy(tests.PoolID(user1ID, pool1)),
		ts.C(user3, server2, server3), tests.FileCSV.Copy(tests.PoolID(user3ID, pool3), tests.Name("a1")),
	)

	tests := []struct {
		c       *Client
		name    string
		text    string
		wantErr error
		want    []hos.Object
		opts    []Options
	}{
		{
			name:    "empty search text",
			text:    "",
			c:       ts.C(user1),
			wantErr: hos.ErrBadRequest,
		},
		{
			name:    "find with admin user",
			text:    "p",
			c:       ts.C(adminUser),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name: "find with admin and with impersonation",
			text: "avi",
			c:    ts.C(adminUser),
			opts: []Options{OnBehalf(user3)},
			want: []hos.Object{*tests.FileAVI.Obj(tests.UserPoolID(user1ID, pool1))},
		},
		{
			name:    "find on all servers with partial user",
			text:    "pool",
			c:       ts.C(user2),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name: "find on all servers with partial user, ignore errors",
			text: "p",
			c:    ts.C(user2),
			opts: []Options{IgnoreErrors(hos.ErrNotExist, hos.ErrNotAuthorized)},
			want: []hos.Object{
				*tests.Object(pool1, tests.ID(tests.PID(user1ID, pool1))),
				*tests.Object(pool2, tests.ID(tests.PID(user2ID, pool2))),
				*tests.Object(pool3, tests.ID(tests.PID(user3ID, pool3))),
			},
		},
		{
			name: "find on all servers, return pool and object at the same time",
			text: "1",
			c:    ts.C(user1),
			want: []hos.Object{
				*tests.FileCSV.Obj(tests.Name("a1"), tests.UserPoolID(user1ID, pool1)),
				*tests.Object(pool1, tests.ID(tests.PID(user1ID, pool1))),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.Find(context.Background(), tt.text, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.Find() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{}), cmpopts.IgnoreFields(hos.Object{},
				"UserID", "Hash", "CreatedAt", "ModifiedAt", "ContentType", "ReplicaCount", "Size", "Labels")); diff != "" {
				t.Errorf("Client.Find() %s", diff)
			}
		})
	}
}
