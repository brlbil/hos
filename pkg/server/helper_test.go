// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/tests"
)

const (
	anonUser  = constant.AnonUser
	adminUser = constant.AdminUser
	everyone  = constant.Everyone

	user1 = tests.User1
	user2 = tests.User2
	user3 = tests.User3

	user1ID = tests.User1ID
	user2ID = tests.User2ID
	user3ID = tests.User3ID

	pool1 = tests.Pool1
	pool2 = tests.Pool2
	pool3 = tests.Pool3
	pool4 = tests.Pool4
	pool5 = tests.Pool5
	pool6 = tests.Pool6
	pool7 = tests.Pool7
	pool8 = tests.Pool8
	pool9 = tests.Pool9
)

var port = &tests.Port{Val: 6000}

func create(c *clientT, t *testing.T, a ...any) {
	t.Helper()
	lena := len(a)
	if lena == 0 {
		t.Fatal("no arguments given")
	}

	user := func(uid string) string {
		switch uid {
		case user1ID:
			return user1
		case user2ID:
			return user2
		case user3ID:
			return user3
		default:
			return ""
		}
	}

	for i := range lena {
		switch o := a[i].(type) {
		case *hos.Pool:
			rsp, _ := c.do(user(o.UserID), put, constant.APIPrefix, o, nil)
			if rsp.StatusCode != 201 {
				t.Fatalf("creating pool %s failed, status %d", o.Name, rsp.StatusCode)
			}
		case *tests.File:
			rsp, _ := c.do(user(o.UserID), put, path.Join(constant.APIPrefix, o.PoolID), o, nil)
			if rsp.StatusCode != 201 {
				t.Fatalf("creating object %s, status %d", o.Name, rsp.StatusCode)
			}
		case map[string]string:
			u := o["user"]
			delete(o, "user")
			rsp, _ := c.do(u, put, constant.KeyAPIPrefix, nil, o)
			if rsp.StatusCode != 201 {
				t.Fatalf("creating enc key failed, status %d", rsp.StatusCode)
			}
		default:
			t.Fatalf("unsported type %T", o)
		}
	}
}

func newTestServer(root string, t *testing.T, users ...string) *Server {
	conf := &Config{
		RootDir:  root,
		Address:  fmt.Sprintf("localhost:%d", port.Next()),
		LogLevel: "none",
	}

	return newTestServerWithConfig(t, conf, users...)
}

func newTestServerWithConfig(t *testing.T, conf *Config, users ...string) *Server {
	t.Helper()

	// clean old files, if any left
	if err := os.RemoveAll(conf.RootDir); err != nil {
		t.Fatal(err)
	}
	// create the test dir
	if err := os.MkdirAll(conf.RootDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// for testing logging creates a lot of noise so we disable it here
	s, err := New(conf)
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = s.Start() }()

	t.Cleanup(func() {
		_ = s.Stop()
		_ = os.RemoveAll(conf.RootDir)
	})

	time.Sleep(time.Millisecond * 3)

	if len(users) == 0 {
		return s
	}

	tc := newTestClient(s, t, adminUser)
	// initialize admin user
	rsp, _ := tc.do(adminUser, post, constant.UserAPIPrefix, tests.User(adminUser), nil)
	if rsp.StatusCode != 204 {
		t.Fatalf("initializing admin user failed, status %d", rsp.StatusCode)
	}
	// create users
	for _, user := range users {
		rsp, _ := tc.do(adminUser, put, constant.UserAPIPrefix, tests.User(user), nil)
		if rsp.StatusCode != 201 {
			t.Fatalf("creating user %s failed, status %d", user, rsp.StatusCode)
		}
	}

	return s
}

type clientT struct {
	tokens map[string]string
	c      *client
}

func newTestClient(s *Server, t *testing.T, users ...string) *clientT {
	t.Helper()

	if len(users) == 0 {
		t.Fatal("at least one user needed")
	}

	tokens := map[string]string{}

	for _, user := range users {
		if user == constant.AnonUser {
			continue
		}
		pk := tests.ParsePrivateKey(user, t)
		tokens[user] = pk.SignUser(user)
	}

	c, err := newClient(s.String(), "")
	if err != nil {
		t.Fatal("newClient", err)
	}

	return &clientT{tokens: tokens, c: c}
}

func (c *clientT) do(user, method, path string, data any, header map[string]string) (*http.Response, error) {
	ctx := context.Background()

	cfns := []clientFunc{headers(header)}
	if data != nil {
		switch i := data.(type) {
		case *hos.User, enc.Key:
			cfns = append(cfns, marshalJSON(i))
		case []byte:
			cfns = append(cfns, uploadBuf(i))
		case *tests.File:
			cfns = append(cfns, marshalHeader(i))
			cfns = append(cfns, uploadFile(i.RelPath()))
		default:
			cfns = append(cfns, marshalHeader(i))
		}
	}
	if tok, ok := c.tokens[user]; ok {
		cfns = append(cfns, setToken(tok))
	}
	qr := strings.Split(path, "?")
	if len(qr) == 2 {
		path = qr[0]
		cfns = append(cfns, setQuery(qr[1]))
	}
	return c.c.do(ctx, method, path, cfns...)
}

func parseResponse[T any](rsp *http.Response) (T, error) {
	var t T
	if rsp.StatusCode >= 400 {
		return t, nil
	}

	switch any(t).(type) {
	case hos.Pool, hos.ServerInfo:
		t2, err := header.Parse[T](rsp.Header)
		return *t2, err
	case hos.Object:
		o, err := header.Parse[hos.Object](rsp.Header)
		o.SetBody(rsp.Body)
		t = any(*o).(T)
		return t, err
	case []hos.Pool, []hos.Object, []hos.User, []hos.Key, []enc.Key, Config:
		if rsp.StatusCode != 200 {
			break
		}
		if err := json.NewDecoder(rsp.Body).Decode(&t); err != nil {
			return t, fmt.Errorf("decoding error: %w", err)
		}
		return t, nil
	}

	return t, nil
}
