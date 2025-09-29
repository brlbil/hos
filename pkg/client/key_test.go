// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestClient_CreateKey(t *testing.T) {
	ts := newTestServer(".create_key", t, createUser(user1), createUser(user2, server1, server3))

	tests := []struct {
		c       *Client
		options []Options
		wantErr error
		name    string
		key     []byte
	}{
		{
			name:    "create with adminUser",
			c:       ts.C(adminUser),
			key:     tests.Key(tests.Key1),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name:    "create with none exist key",
			c:       ts.C(user1),
			key:     tests.Key(tests.Key1),
			options: []Options{EncryptionKey(tests.Key(tests.Key1))},
			wantErr: hos.ErrNotExist,
		},
		{
			name: "create one",
			c:    ts.C(user1),
			key:  tests.Key(tests.Key1),
		},
		{
			name:    "create two without two",
			c:       ts.C(user1),
			key:     tests.Key(tests.Key2),
			wantErr: hos.ErrExist,
		},
		{
			name:    "create two with one",
			c:       ts.C(user1),
			key:     tests.Key(tests.Key2),
			options: []Options{EncryptionKey(tests.Key(tests.Key1))},
		},
		{
			name:    "if not all servers available fails",
			c:       ts.C(user2),
			key:     tests.Key(tests.Key2),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name:    "create wrong key with two",
			c:       ts.C(user1),
			key:     tests.Key(tests.ShortKey),
			options: []Options{EncryptionKey(tests.Key(tests.Key2))},
			wantErr: hos.ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.CreateKey(context.Background(), tt.key, tt.options...); !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.CreateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_RestoreKey(t *testing.T) {
	ts := newTestServer(".restore_key", t, createUser(user1), createUser(user2))

	create[string](t,
		ts.C(user1), tests.Key1,
		ts.C(user1), tests.Key2+tests.Key1,
		ts.C(user2, server1, server3), tests.Key1,
		ts.C(user2, server1, server3), tests.Key2+tests.Key1,
	)

	ctx := context.Background()

	// backup keys
	user1Keys, err := ts.C(user1).GetServerKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user2Keys, err := ts.C(user2).GetServerKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}

	user1Key := tests.UserKey(user1, tests.Key1)
	user2Key := tests.UserKey(user2, tests.Key1)
	// remove keys1
	if err := ts.C(user1).DeleteKey(ctx, user1Key.Signature); err != nil {
		t.Fatal(err)
	}
	if err := ts.C(user2, server1, server3).DeleteKey(ctx, user2Key.Signature); err != nil {
		t.Fatal(err)
	}

	server1Addr := ts.ss[0].String()
	server2Addr := ts.ss[1].String()
	server3Addr := ts.ss[2].String()

	tests := []struct {
		c       *Client
		name    string
		keyID   string
		data    map[string]enc.Key
		wantErr error
		opts    []Options
		want    []hos.Key
	}{
		{
			name:  "restore key1 user1",
			c:     ts.C(user1),
			keyID: user1Key.Signature,
			data: map[string]enc.Key{
				server1Addr: user1Keys[server1Addr][0],
				server2Addr: user1Keys[server2Addr][0],
				server3Addr: user1Keys[server3Addr][0],
			},
			want: []hos.Key{
				tests.UserKey(user1, tests.Key1),
				tests.UserKey(user1, tests.Key2),
			},
		},
		{
			name:  "restore key1 user2",
			c:     ts.C(user2),
			keyID: user2Key.Signature,
			data: map[string]enc.Key{
				server1Addr: user2Keys[server1Addr][0],
				server3Addr: user2Keys[server3Addr][0],
			},
			want: []hos.Key{
				tests.UserKey(user2, tests.Key1),
				tests.UserKey(user2, tests.Key2),
			},
			// used with listing keys
			opts: []Options{WarnErrors(hos.ErrNotExist)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.RestoreKey(ctx, tt.data)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.RestoreKey() error = %v, wantErr %v", err, tt.wantErr)
			}

			keys, err := tt.c.ListKeys(ctx, tt.opts...)
			if err != nil {
				t.Errorf("Client.RestoreKey() error = %v", err)
			}

			if diff := cmp.Diff(keys, tt.want, cmpopts.IgnoreFields(hos.Key{}, "CreatedAt")); diff != "" {
				t.Errorf("Client.RestoreKey() %s", diff)
			}
		})
	}
}

func TestClient_ListKeys(t *testing.T) {
	ts := newTestServer(".list_keys", t, createUser(user1), createUser(user2))

	create[string](t,
		ts.C(user1), tests.Key1,
		ts.C(user1), tests.Key2+tests.Key1,
		ts.C(user2, server1, server3), tests.Key1,
	)

	tests := []struct {
		c       *Client
		name    string
		wantErr error
		opts    []Options
		want    []hos.Key
	}{
		{
			name: "list keys user1",
			c:    ts.C(user1),
			want: []hos.Key{
				tests.UserKey(user1, tests.Key1),
				tests.UserKey(user1, tests.Key2),
			},
		},
		{
			name:    "admin user not allowed",
			c:       ts.C(adminUser),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name:    "list keys user2",
			c:       ts.C(user2),
			wantErr: hos.ErrNotExist,
		},
		{
			name: "list keys user2 with warnErr",
			c:    ts.C(user2),
			opts: []Options{WarnErrors(hos.ErrNotExist)},
			want: []hos.Key{tests.UserKey(user2, tests.Key1)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.ListKeys(context.Background(), tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.ListKeys() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreFields(hos.Key{}, "CreatedAt")); diff != "" {
				t.Errorf("Client.ListKeys() %s", diff)
			}
		})
	}
}

func TestClient_GetServerKeys(t *testing.T) {
	ts := newTestServer(".server_keys", t, createUser(user1), createUser(user2))

	create[string](t,
		ts.C(user1), tests.Key1,
		ts.C(user1), tests.Key2+tests.Key1,
		ts.C(user2, server1, server3), tests.Key1,
	)

	tests := []struct {
		c       *Client
		name    string
		wantErr error
		opts    []Options
		want    map[string][]enc.Key
	}{
		{
			name: "get keys user1",
			c:    ts.C(user1),
			want: map[string][]enc.Key{
				ts.ss[0].String(): {
					tests.UserKeyData(user1, tests.Key1),
					tests.UserKeyData(user1, tests.Key2),
				},
				ts.ss[1].String(): {
					tests.UserKeyData(user1, tests.Key1),
					tests.UserKeyData(user1, tests.Key2),
				},
				ts.ss[2].String(): {
					tests.UserKeyData(user1, tests.Key1),
					tests.UserKeyData(user1, tests.Key2),
				},
			},
		},
		{
			name:    "admin user not allowed",
			c:       ts.C(adminUser),
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name: "get keys user2",
			c:    ts.C(user2),
			want: map[string][]enc.Key{
				ts.ss[0].String(): {
					tests.UserKeyData(user2, tests.Key1),
				},
				ts.ss[1].String(): {},
				ts.ss[2].String(): {
					tests.UserKeyData(user2, tests.Key1),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.GetServerKeys(context.Background(), tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.GetServerKeys() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(got, tt.want,
				cmpopts.IgnoreFields(enc.Key{}, "CreatedAt"),
				cmpopts.IgnoreUnexported(enc.Key{}),
				cmp.Transformer("bytesLen", func(b []byte) int { return len(b) }),
			); diff != "" {
				t.Errorf("Client.GetServerKeys() %s", diff)
			}
		})
	}
}

func TestClient_DeleteKey(t *testing.T) {
	ts := newTestServer(".delete_key", t, createUser(user1), createUser(user2))

	create[string](t,
		ts.C(user1), tests.Key1,
		ts.C(user1, server1, server2), tests.Key2+tests.Key1,
		ts.C(user2, server1, server3), tests.Key1,
	)

	keys := []hos.Key{
		tests.UserKey(user1, tests.Key1),
		tests.UserKey(user1, tests.Key2),
		tests.UserKey(user2, tests.Key1),
		tests.UserKey(user1, tests.Key3),
	}

	tests := []struct {
		c       *Client
		wantErr error
		name    string
		kid     string
	}{
		{
			name:    "deleting with admin user",
			c:       ts.C(adminUser),
			kid:     "ksjdhfkjsdhf", // some random id
			wantErr: hos.ErrNotAuthorized,
		},
		{
			name:    "not exist key",
			c:       ts.C(user1),
			kid:     keys[3].Signature,
			wantErr: hos.ErrNotExist,
		},
		{
			name:    "partial key",
			c:       ts.C(user2),
			kid:     keys[2].Signature,
			wantErr: hos.ErrNotAllowed,
		},
		{
			name: "delete key two",
			c:    ts.C(user1),
			kid:  keys[1].Signature,
		},
		{
			name:    "delete the last key",
			c:       ts.C(user1),
			kid:     keys[0].Signature,
			wantErr: hos.ErrNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.c.DeleteKey(context.Background(), tt.kid); !errors.Is(err, tt.wantErr) {
				t.Errorf("Client.DeleteKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
