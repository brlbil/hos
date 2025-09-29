// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/tests"
	"github.com/brlbil/hos/pkg/server"
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

	read  = tests.Read
	write = tests.Write

	tooLargeSize = 1024 * 1024 * 1024 * 1024 * 1024
)

type serverID int

const (
	server1 serverID = iota
	server2
	server3
	server4
)

var port = &tests.Port{Val: 5000}

var encKey []byte

func setEncKey(b []byte) tests.Option {
	return func(a any) {
		encKey = b
	}
}

type serverOption func(*testing.T, []ServerConfig) (string, []ServerConfig)

func createUser(name string, serverIDs ...serverID) serverOption {
	return func(t *testing.T, sc []ServerConfig) (string, []ServerConfig) {
		t.Helper()
		if len(serverIDs) == 0 {
			return name, sc[:3]
		}

		serverConfigs := []ServerConfig{}
		for _, sid := range serverIDs {
			if sid > 3 || sid < 0 {
				t.Fatal("index cannot be less than 0, or greater than 3")
			}
			serverConfigs = append(serverConfigs, sc[int(sid)])
		}

		return name, serverConfigs
	}
}

func create[T string | hos.Pool | tests.File](t *testing.T, a ...any) {
	t.Helper()

	lena := len(a)
	if lena == 0 {
		t.Fatal("no arguments given")
	}

	even := lena % 2
	if even != 0 {
		t.Fatalf("arguments must be even numbers, len %d", lena)
	}

	for i := 0; i < lena; i += 2 {
		c, ok := a[i].(*Client)
		if !ok {
			t.Fatalf("expected %d argument type as client, pairs must be first client, and second the type", i)
		}

		var tt T
		_, isString := any(tt).(string)

		if !isString {
			_, ok = a[i+1].(*T)
			if !ok {
				t.Fatalf("%d argument expected to be type %T", i+1, tt)
			}
		}

		switch o := a[i+1].(type) {
		case *hos.Pool:
			_, err := c.CreatePool(context.Background(), o)
			if err != nil {
				t.Fatalf("creating pool %s failed: %s", o.Name, err)
			}
		case *tests.File:
			opts := []Options{}
			// this is a bit problemetic when we set the encKey
			// regardless which File it is set for it is used by the first File from create func
			// so encrypted files needs to be in front
			if len(encKey) > 0 {
				opts = append(opts, EncryptionKey(encKey))
				encKey = nil
			}
			_, err := c.CreateObject(context.Background(), o.Obj(), o, opts...)
			if err != nil {
				t.Fatalf("creating object %s failed: %s", o.Name, err)
			}
		// this creates keys
		case string:
			oo := []Options{}
			if len(o) > 45 {
				in := strings.Index(o, "=")
				sec := o[in+1:]
				oo = append(oo, EncryptionKey(tests.Key(sec)))
				o = o[:in+1]
			}
			if err := c.CreateKey(context.Background(), tests.Key(o), oo...); err != nil {
				t.Fatalf("creating key failed: %s", err)
			}
		}
	}
}

// testServer implements multiple api/servers,
// it has 3 running and one not running server
type testServer struct {
	ss    []*server.Server
	confs []ServerConfig
}

func (ts *testServer) C(user string, servers ...serverID) *Client {
	sc := []ServerConfig{}
	if len(servers) == 0 {
		servers = []serverID{server1, server2, server3}
	}
	for _, i := range servers {
		sc = append(sc, ts.confs[i])
	}
	opts := []ConfigFunc{notUseHTTP2}
	if user != anonUser {
		opts = append(opts, SetUserKey(user, tests.PrivateKey(user)))
	}
	c, _ := New(sc, opts...)
	return c
}

func newTestServer(root string, t *testing.T, opts ...serverOption) *testServer {
	t.Helper()
	// clean old files, if any left
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}

	s := &testServer{ss: []*server.Server{}, confs: []ServerConfig{}}
	t.Cleanup(func() {
		for _, srv := range s.ss[:3] {
			_ = srv.Stop()
		}
		_ = os.RemoveAll(root)
	})

	for i := 1; i < 5; i++ {
		sd := filepath.Join(root, fmt.Sprintf("server%d", i))
		// create the server dir
		if err := os.MkdirAll(sd, 0o750); err != nil {
			t.Fatal(err)
		}

		conf := &server.Config{
			RootDir:  sd,
			Address:  fmt.Sprintf("localhost:%d", port.Next()),
			LogLevel: "none",
		}

		srv, err := server.New(conf)
		if err != nil {
			t.Fatal(err)
		}

		if i != 4 {
			go func() { _ = srv.Start() }()
		}
		time.Sleep(time.Millisecond * 3)

		s.ss = append(s.ss, srv)
		caFile := filepath.Join(sd, ".certs", "ca.pem")
		ca, err := os.ReadFile(caFile)
		if err != nil {
			t.Fatalf("failed ot read ca fle %s, %s", caFile, err)
		}
		s.confs = append(s.confs, ServerConfig{Address: srv.String(), Certificate: string(ca)})
	}

	if len(opts) == 0 {
		return s
	}

	// if there are users then first create admin user
	ac, err := New(s.confs[:3], SetUserKey(adminUser, tests.PrivateKey(adminUser)), notUseHTTP2)
	if err != nil {
		t.Fatalf("creating new client failed: %s", err)
	}
	if err := ac.EditUser(context.Background(), tests.User(adminUser)); err != nil {
		t.Fatalf("initializing admin user failed: %s", err)
	}

	for _, opt := range opts {
		user, conf := opt(t, s.confs)
		c, err := New(conf, SetUserKey(adminUser, tests.PrivateKey(adminUser)), notUseHTTP2)
		if err != nil {
			t.Fatal(err)
		}

		uu := tests.User(user)
		if err := c.CreateUser(context.Background(), uu); err != nil {
			t.Fatal(err)
		}
	}

	return s
}
