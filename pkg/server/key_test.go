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

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestServer_Keys(t *testing.T) {
	srv := newTestServer(".test_keys", t, user1, user2)

	tc := newTestClient(srv, t, adminUser, user1, user2)

	tests := []struct {
		name     string
		user     string
		method   string
		path     string
		headers  map[string]string
		wantKeys []hos.Key
		wantCode int
	}{
		// add key admin client, admin cannot set enc key
		{
			name:     "crt1",
			user:     adminUser,
			method:   put,
			wantCode: 401,
		},
		// create key with user1 client, try to create a key with none exist key
		{
			name:     "crt2",
			user:     user1,
			method:   put,
			wantCode: 404,
			headers: tests.Map[string](
				header.EncryptionNewKey, tests.Key2,
				header.EncryptionKey, tests.Key1,
			),
		},
		// create key1 with user1 client
		{
			name:     "crt3",
			user:     user1,
			method:   put,
			wantCode: 201,
			headers:  tests.Map[string](header.EncryptionNewKey, tests.Key1),
		},
		// create key2 with user1 client, despite there being key1
		{
			name:     "crt4",
			user:     user1,
			method:   put,
			wantCode: 409,
			headers:  tests.Map[string](header.EncryptionNewKey, tests.Key2),
		},
		// create key2 with user1 client
		{
			name:     "crt5",
			user:     user1,
			method:   put,
			wantCode: 201,
			headers: tests.Map[string](
				header.EncryptionNewKey, tests.Key2,
				header.EncryptionKey, tests.Key1,
			),
		},
		// create key2 again with user1 client
		{
			name:     "crt6",
			user:     user1,
			method:   put,
			wantCode: 409,
			headers: tests.Map[string](
				header.EncryptionNewKey, tests.Key2,
				header.EncryptionKey, tests.Key1,
			),
		},
		// create key3 with user1 client
		{
			name:     "crt7",
			user:     user1,
			method:   put,
			wantCode: 201,
			headers: tests.Map[string](
				header.EncryptionNewKey, tests.Key3,
				header.EncryptionKey, tests.Key2,
			),
		},
		// get keys with user1
		{
			name:     "get1",
			user:     user1,
			method:   get,
			wantCode: 200,
			wantKeys: []hos.Key{
				tests.UserKey(user1, tests.Key1),
				tests.UserKey(user1, tests.Key2),
				tests.UserKey(user1, tests.Key3),
			},
		},
		// get keys with user2
		{
			name:     "get2",
			user:     user2,
			method:   get,
			wantCode: 200,
			wantKeys: []hos.Key{},
		},
		// delete key1 user1
		{
			name:     "del1",
			user:     user1,
			method:   del,
			path:     tests.UserKey(user1, tests.Key1).Signature,
			wantCode: 204,
		},
		// delete key1 again user1
		{
			name:     "del2",
			user:     user1,
			method:   del,
			path:     tests.UserKey(user1, tests.Key1).Signature,
			wantCode: 404,
		},
		// delete key3 again user1
		{
			name:     "del3",
			user:     user1,
			method:   del,
			path:     tests.UserKey(user1, tests.Key3).Signature,
			wantCode: 204,
		},
		// get keys again with user1
		{
			name:     "get3",
			user:     user1,
			method:   get,
			wantCode: 200,
			wantKeys: []hos.Key{tests.UserKey(user1, tests.Key2)},
		},
		// delete remaning key2 user1
		{
			name:     "del4",
			user:     user1,
			method:   del,
			path:     tests.UserKey(user1, tests.Key2).Signature,
			wantCode: 444,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := path.Join(constant.KeyAPIPrefix, tt.path)
			rsp, err := tc.do(tt.user, tt.method, path, nil, tt.headers)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				msg, _ := io.ReadAll(rsp.Body)
				t.Errorf("Expected %s(%d), got %s(%d), msg: %s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode, string(msg))
			}

			got, err := parseResponse[[]hos.Key](rsp)
			if err != nil {
				t.Error(err)
			}

			if diff := cmp.Diff(got, tt.wantKeys, cmpopts.IgnoreFields(hos.Key{}, "CreatedAt")); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestServer_KeyRestore(t *testing.T) {
	srv := newTestServer(".test_restore", t, user1)
	tc := newTestClient(srv, t, adminUser, user1)

	create(tc, t,
		tests.Pool(pool1, tests.UserID(user1ID), tests.Encrypted()),
		map[string]string{"user": user1, header.EncryptionNewKey: tests.Key1},                                   // create key1 enc key
		map[string]string{"user": user1, header.EncryptionKey: tests.Key1, header.EncryptionNewKey: tests.Key2}, // create key2 enc key
	)

	key1 := tests.UserKey(user1, tests.Key1)
	var keyData []enc.Key

	tests := []struct {
		name     string
		method   string
		path     string
		data     any
		headers  map[string]string
		wantCode int
		postFn   func(*testing.T, *http.Response)
	}{
		{
			name:     "backup key",
			method:   get,
			path:     constant.KeyAPIDataPrefix,
			wantCode: 200,
			postFn: func(t *testing.T, rsp *http.Response) {
				got, err := parseResponse[[]enc.Key](rsp)
				if err != nil {
					t.Error(err)
				}
				keyData = got
			},
		},
		{
			name:     "upload file",
			method:   put,
			path:     path.Join(constant.APIPrefix, tests.PID(user1ID, pool1)),
			data:     tests.FileAVI.Copy(),
			headers:  map[string]string{header.EncryptionKey: tests.Key1},
			wantCode: 201,
		},
		{
			name:     "delete key",
			method:   del,
			path:     path.Join(constant.KeyAPIPrefix, key1.Signature),
			wantCode: 204,
		},
		{
			name:     "restore key1",
			method:   put,
			path:     constant.KeyAPIDataPrefix,
			wantCode: 201,
		},
		{
			name:     "list keys",
			method:   get,
			path:     constant.KeyAPIPrefix,
			wantCode: 200,
			postFn: func(t *testing.T, rsp *http.Response) {
				keys, err := parseResponse[[]hos.Key](rsp)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.ContainsFunc(keys, func(k hos.Key) bool {
					return k.Signature == key1.Signature
				}) {
					t.Errorf("key1 is not exists")
				}
			},
		},
		{
			name:     "get file with key1",
			method:   get,
			path:     path.Join(constant.APIPrefix, tests.PID(user1ID, pool1), tests.OID(user1ID, pool1, tests.AVI)),
			headers:  map[string]string{header.EncryptionKey: tests.Key1},
			wantCode: 200,
			postFn: func(t *testing.T, rsp *http.Response) {
				if rsp.ContentLength != tests.FileAVI.Size {
					t.Errorf("Length expected %d, got %d", tests.FileAVI.Size, rsp.ContentLength)
				}

				hw := sha256.New()
				_, _ = io.ReadAll(io.TeeReader(rsp.Body, hw))

				if hash := fmt.Sprintf("%x", hw.Sum(nil)); hash != tests.FileAVI.Hash {
					t.Errorf("hash expected %s, got %s", tests.FileAVI.Hash, hash)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.method == put && tt.data == nil {
				tt.data = keyData[0]
			}
			rsp, err := tc.do(user1, tt.method, tt.path, tt.data, tt.headers)
			if err != nil {
				t.Fatal(err)
			}

			if rsp.StatusCode != tt.wantCode {
				msg, _ := io.ReadAll(rsp.Body)
				t.Errorf("Expected %s(%d), got %s(%d), msg: %s",
					http.StatusText(tt.wantCode), tt.wantCode,
					http.StatusText(rsp.StatusCode), rsp.StatusCode, string(msg))
			}

			if tt.postFn != nil {
				tt.postFn(t, rsp)
			}
		})
	}
}
