// SPDX-License-Identifier: MIT

package fs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/logger"
	"github.com/brlbil/hos/internal/xattr"
	"github.com/brlbil/hos/pkg/crypto"
	"github.com/brlbil/hos/pkg/id"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/minio/sio"
)

func mkTestDir(path string, t *testing.T) *FS {
	t.Helper()
	if _, err := xattr.List("."); err != nil {
		t.Skip("Do not support xattr")
	}

	t.Cleanup(func() { os.RemoveAll(path) })

	for _, dir := range []string{"", "a59d1943"} {
		if err := os.MkdirAll(filepath.Join(path, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}

	for _, f := range []string{"f590e335", "some_file"} {
		if err := os.WriteFile(filepath.Join(path, f), []byte{0}, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	log, err := logger.New("fs-test", logger.WithLevel("none"))
	if err != nil {
		t.Fatal(err)
	}

	fs, err := New(path, log.Logger)
	if err != nil {
		t.Fatal(err)
	}

	return fs
}

func TestUser(t *testing.T) {
	fs := mkTestDir(".users", t)

	ctx := context.Background()

	u := &hos.User{Name: "user", ID: "8d93d649", PublicKeys: []crypto.PublicKey{[]byte{7, 4, 8, 3, 5, 7, 0, 3, 5}}}
	if err := fs.CreateUser(ctx, u); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	u2, err := fs.GetUser(ctx, u.ID)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff(u, u2, cmpopts.IgnoreUnexported(hos.User{})); diff != "" {
		t.Error(diff)
	}

	users, err := fs.GetUsers(ctx)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff([]hos.User{*u}, users, cmpopts.IgnoreUnexported(hos.User{})); diff != "" {
		t.Error(diff)
	}

	if err := fs.DeleteUser(ctx, u.ID); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if _, err := fs.GetUsers(ctx); err == nil {
		t.Error("expected error got nil")
	}
}

func TestKey(t *testing.T) {
	fs := mkTestDir(".keys", t)

	ctx := context.Background()

	k, err := enc.Create("8d93d649", "diwGg/krPw9nG5JuFLOT/zeJY/+3zjBmuioZV9DJxTw=")
	if err != nil {
		t.Fatal(err)
	}
	// copy k for later
	k1 := *k

	if err := fs.CreateKey(ctx, k); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	k2, err := fs.GetKey(ctx, k.ID)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff(&k1, k2, cmpopts.IgnoreUnexported(enc.Key{})); diff != "" {
		t.Error(diff)
	}

	if cmp.Equal(k, k2, cmpopts.IgnoreUnexported(enc.Key{})) {
		t.Errorf("expected to be different, %v", k)
	}

	if err := fs.DeleteKey(ctx, k.ID); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if _, err := fs.GetKey(ctx, k.ID); err == nil {
		t.Error("expected error got nil")
	}
}

func TestPool(t *testing.T) {
	fs := mkTestDir(".pool", t)

	ctx := context.Background()

	p := &hos.Pool{
		Name: "pool1",
		ID:   "d5479175", UserID: "8d93d649",
		ReplicaCount: 1,
		Labels:       map[string]string{"L": "V"},
		Permissions:  map[string]hos.Permission{"*": "r"},
	}
	if err := fs.CreatePool(ctx, p); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	p2, err := fs.GetPool(ctx, p.ID)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff(p, p2); diff != "" {
		t.Error(diff)
	}

	if err := fs.DeletePool(ctx, p.ID); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if _, err := fs.GetPool(ctx, p.ID); err == nil {
		t.Error("expected error got nil")
	}
}

func TestObject(t *testing.T) {
	fs := mkTestDir(".object", t)

	ctx := context.Background()
	p := &hos.Pool{
		Name:         "pool1",
		ID:           "d5479175",
		UserID:       "8d93d649",
		ReplicaCount: 1,
		Labels:       map[string]string{"L": "V"},
		Permissions:  map[string]hos.Permission{"*": "r"},
	}
	if err := fs.CreatePool(ctx, p); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	data := []byte{7, 4, 7, 8, 3, 2, 5, 6, 8, 9, 3, 2, 5, 7, 88, 3, 22, 123, 43, 87}
	o := &hos.Object{
		Name: "obj1", ID: "976d4705", PoolID: "d5479175", UserID: "8d93d649", ReplicaCount: 1, ContentType: "application/data",
		Hash: "877839f48aaff85e906734f3b0616d3358877da7d95193ddb7aa74a09075073b", Size: 20, Labels: map[string]string{"L": "V"},
	}

	if err := fs.CreateObject(ctx, o, bytes.NewReader(data), nil); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	o2, err := fs.GetObject(ctx, o.PoolID, o.ID)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff(o, o2,
		cmpopts.IgnoreUnexported(hos.Object{}),
		cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt"),
	); diff != "" {
		t.Error(diff)
	}

	rc, err := fs.ReadObject(ctx, o, nil)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}
	defer rc.Close()

	data2, _ := io.ReadAll(rc)
	if diff := cmp.Diff(data, data2); diff != "" {
		t.Error(diff)
	}

	if err := fs.DeleteObject(ctx, o.PoolID, o.ID); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if _, err := fs.GetObject(ctx, o.PoolID, o.ID); err == nil {
		t.Error("expected error got nil")
	}

	// create it again
	if err := fs.CreateObject(ctx, o, bytes.NewReader(data), nil); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	p2 := &hos.Pool{Name: "pool2", ID: "9a70d69c", UserID: "8d93d649", ReplicaCount: 1}
	if err := fs.CreatePool(ctx, p2); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	o.Name = "obj0"
	if err := fs.MoveObject(ctx, o, p2.ID); err != nil {
		t.Errorf("move expected error <nil>, got %s", err)
	}

	o3 := *o
	o3.PoolID = p2.ID
	o3.ID = id.Gen(p2.ID, "obj0")

	o4, err := fs.GetObject(ctx, p2.ID, o3.ID)
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff(&o3, o4,
		cmpopts.IgnoreUnexported(hos.Object{}),
		cmpopts.IgnoreFields(hos.Object{}, "CreatedAt", "ModifiedAt"),
	); diff != "" {
		t.Error(diff)
	}

	// test with encryption
	ec := &sio.Config{
		MinVersion:   sio.Version20,
		MaxVersion:   sio.Version20,
		CipherSuites: []byte{sio.CHACHA20_POLY1305},
		Key:          bytes.Repeat([]byte{67, 68, 99, 121}, 8),
	}

	o2.Encrypted = true
	if err := fs.CreateObject(ctx, o2, bytes.NewReader(data), ec); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	rc, err = fs.ReadObject(ctx, o, nil)
	if err != nil {
		t.Fatalf("expected error <nil>, got %s", err)
	}
	defer rc.Close()

	data2, _ = io.ReadAll(rc)
	if diff := cmp.Diff(data, data2); diff == "" {
		t.Error(diff)
	}

	rc, err = fs.ReadObject(ctx, o, ec)
	if err != nil {
		t.Fatalf("expected error <nil>, got %s", err)
	}
	defer rc.Close()

	data2, _ = io.ReadAll(rc)
	if diff := cmp.Diff(data, data2); diff != "" {
		t.Error(diff)
	}
}
