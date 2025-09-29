// SPDX-License-Identifier: MIT

// Package fs provides filesystem operations for HOS storage management.
// It handles pools, objects, users, and keys with metadata as extended attributes.
package fs

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/utils"
	"github.com/brlbil/hos/internal/xattr"
	"github.com/brlbil/hos/pkg/id"
	"github.com/minio/sio"
	"golang.org/x/sys/unix"
)

const keyPath = ".keys"

// FS represents a filesystem-based storage backend for storing
// pools, objects, users, and keys. Metadata is stored as filesystem extended attributes.
// Pools are created as directories, objects as files in pool directories,
// users, and keys also as files.
type FS struct {
	root string
	log  *slog.Logger
}

// New creates a new filesystem storage instance
func New(root string, log *slog.Logger) (*FS, error) {
	log = log.With("lib", "fs")

	// test if the fs has xattrs
	log.Debug("creating new FS object", "path", root)
	_, err := xattr.List(root)
	if err != nil {
		return nil, err
	}

	fs := &FS{root: root, log: log}
	if err := os.MkdirAll(fs.path(keyPath), 0o750); err != nil {
		return nil, err
	}

	return fs, nil
}

func (f *FS) path(paths ...string) string {
	return filepath.Join(f.root, filepath.Join(paths...))
}

// GetUsers retrieves all users from the filesystem
func (f *FS) GetUsers(ctx context.Context) ([]hos.User, error) {
	f.log.Debug("reading user files")
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f.log.Debug("reading root directory", "path", f.root)
	dirEntries, err := os.ReadDir(f.root)
	if err != nil {
		return nil, fmt.Errorf("reading directory failed: %w", err)
	}

	users := []hos.User{}
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() || len(dirEntry.Name()) != 8 {
			// we do not care about directories or file name is not equal to 8
			f.log.Debug("ignoring file entry", "entry_name", dirEntry.Name(), "dir", dirEntry.IsDir())
			continue
		}

		user, err := xattr.Get[hos.User](f.path(dirEntry.Name()))
		if err != nil {
			f.log.Debug("reading metadata", "entry_name", dirEntry.Name(), "error", err)
			continue
		}
		users = append(users, *user)
	}

	if len(users) == 0 {
		return users, hos.ErrNotExist
	}

	return users, nil
}

// GetUser retrieves a specific user by ID
func (f *FS) GetUser(ctx context.Context, uid string) (*hos.User, error) {
	f.log.Debug("creating user", "user_id", uid)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return xattr.Get[hos.User](f.path(uid))
}

// GetKey retrieves an encryption key by ID
func (f *FS) GetKey(ctx context.Context, kid string) (*enc.Key, error) {
	f.log.Debug("getting user", "key_id", kid)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	keyPath := f.path(keyPath, kid)
	key, err := xattr.Get[enc.Key](keyPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	key.Data = data

	return key, nil
}

// GetPool retrieves a pool by ID
func (f *FS) GetPool(ctx context.Context, pid string) (*hos.Pool, error) {
	f.log.Debug("getting pool", "pool_id", pid)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return xattr.Get[hos.Pool](f.path(pid))
}

// GetObject retrieves an object by pool and object ID
func (f *FS) GetObject(ctx context.Context, pid, oid string) (*hos.Object, error) {
	f.log.Debug("getting object", "pool_id", pid, "object_id", oid)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return xattr.Get[hos.Object](f.path(pid, oid))
}

// ReadObject opens an object file for reading with optional encryption support
func (f *FS) ReadObject(ctx context.Context, object *hos.Object, encryptionConfig *sio.Config) (io.ReadSeekCloser, error) {
	f.log.Debug("reading object", "pool_id", object.PoolID, "object_id", object.ID, "object_name", object.Name)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	filePath := filepath.Join(f.root, object.PoolID, object.ID)
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s %w", filePath, hos.ErrNotExist)
		}
		return nil, err
	}

	readSeekCloser, err := newEncReader(file, object.Size, encryptionConfig, f.log)
	if err != nil {
		return nil, err
	}

	if encryptionConfig != nil && object.Size > 8 {
		if _, err := io.CopyN(io.Discard, readSeekCloser, 8); err != nil {
			return nil, errors.Join(err, hos.ErrDecryption)
		}
		_, _ = readSeekCloser.Seek(0, io.SeekStart)
	}

	return readSeekCloser, nil
}

// CreateUser creates a new user file with stores metadata as xattr
func (f *FS) CreateUser(ctx context.Context, user *hos.User) error {
	f.log.Debug("creating user", "user_id", user.ID, "user_name", user.Name)
	if err := ctx.Err(); err != nil {
		return err
	}

	userFilePath := f.path(user.ID)
	if _, err := os.Stat(userFilePath); err == nil {
		return hos.ErrExist
	}

	userFile, err := os.OpenFile(userFilePath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	userFile.Close()

	return xattr.Set(f.path(user.ID), user)
}

// CreateKey creates a new encryption key file with metadata as xattr
func (f *FS) CreateKey(ctx context.Context, key *enc.Key) error {
	f.log.Debug("creating key", "key_id", key.ID)
	if err := ctx.Err(); err != nil {
		return err
	}

	keyFilePath := f.path(keyPath, key.ID)
	if _, err := os.Stat(keyFilePath); err == nil {
		return hos.ErrExist
	}
	if err := os.WriteFile(keyFilePath, key.Data, 0o600); err != nil {
		return err
	}
	key.Data = nil

	return xattr.Set(keyFilePath, key)
}

// CreatePool creates a new pool directory with metadata as xattr
func (f *FS) CreatePool(ctx context.Context, p *hos.Pool) error {
	f.log.Debug("creating pool", "pool_id", p.ID, "pool_name", p.Name)
	if err := ctx.Err(); err != nil {
		return err
	}

	p2, err := xattr.Get[hos.Pool](f.path(p.ID))
	if err == nil {
		if err := utils.Diff(p, p2); err != nil {
			return errors.Join(err, hos.ErrNotEqual)
		}
		return hos.ErrExist
	}

	// check the destination
	if p.LinkedID != "" {
		f.log.Debug("checking linked pool", "linked_pool_id", p.LinkedID)
		pd, err := xattr.Get[hos.Pool](f.path(p.LinkedID))
		if err != nil {
			// reading destination fails, most likely it is not exist
			return err
		}

		// if the linked pool is also a linked pool, then the operation is not allowed
		if pd.LinkedID != "" {
			return hos.ErrNotAllowed
		}
	}

	dp := filepath.Join(f.root, p.ID)

	var fe error
	defer func() {
		if fe != nil {
			if err := os.Remove(dp); err != nil {
				f.log.Error("removing directory", "error", err, "directory", dp)
			}
		}
	}()

	f.log.Debug("creating pool directory", "path", dp)
	fe = os.Mkdir(dp, 0o750)
	if fe != nil {
		return fe
	}

	fe = xattr.Set(f.path(p.ID), p)
	return fe
}

// CreateObject creates a new object file with optional encryption and metadata as xattr
func (f *FS) CreateObject(ctx context.Context, o *hos.Object, r io.Reader, encConf *sio.Config) error {
	f.log.Debug("creating object", "pool_id", o.PoolID, "object_id", o.ID, "object_name", o.Name)
	if err := ctx.Err(); err != nil {
		return err
	}

	path := filepath.Join(f.root, o.PoolID, o.ID)
	f.log.Debug("checking file stat", "path", path)
	if _, err := os.Stat(path); err == nil {
		return hos.ErrExist
	}

	f.log.Debug("creating file", "path", path)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	sizeUnknown := o.Size == -123
	countWriter := utils.CountWriter(0)

	hash := sha256.New()
	// file hash calculated for not encrypted data
	var er io.Reader
	if sizeUnknown {
		er = io.TeeReader(r, io.MultiWriter(hash, &countWriter))
	} else {
		er = io.TeeReader(r, hash)
	}

	if encConf != nil {
		f.log.Debug("getting encryption reader")
		er, err = sio.EncryptReader(er, *encConf)
		if err != nil {
			return err
		}
	}

	var fe error
	defer func() {
		if fe != nil {
			if err := os.Remove(path); err != nil {
				f.log.Error("removing file", "error", err, "path", path)
			}
		}
	}()

	f.log.Debug("coping data to file", "path", path)
	size, err := io.Copy(file, er)
	if err != nil {
		fe = err
		return err
	}

	if sizeUnknown {
		o.Size = int64(countWriter)
		f.log.Debug("setting object size", "path", path, "size", o.Size)
	} else {
		f.log.Debug("checking written file size", "path", path)
		// if encryption is enabled size cannot be checked
		if !o.Encrypted && size != o.Size {
			fe = fmt.Errorf("written amount %d, not match content size %d", size, o.Size)
			return fe
		}
	}

	// if object's hash is set, than we keep the original hash'
	if o.Hash == "" {
		// set the hash
		o.Hash = fmt.Sprintf("%x", hash.Sum(nil))
	}

	fe = xattr.Set(f.path(o.PoolID, o.ID), o)
	return fe
}

// UpdateUser updates user metadata in extended attributes
func (f *FS) UpdateUser(ctx context.Context, u *hos.User) error {
	f.log.Debug("updating user", "user_id", u.ID, "user_name", u.Name)
	if err := ctx.Err(); err != nil {
		return err
	}

	return xattr.Set(f.path(u.ID), u)
}

// UpdatePool updates pool metadata in extended attributes
func (f *FS) UpdatePool(ctx context.Context, p *hos.Pool) error {
	f.log.Debug("updating pool", "pool_id", p.ID, "pool_name", p.Name)
	if err := ctx.Err(); err != nil {
		return err
	}

	return xattr.Set(f.path(p.ID), p)
}

// UpdateObject updates object metadata in extended attributes
func (f *FS) UpdateObject(ctx context.Context, o *hos.Object) error {
	f.log.Debug("updating object", "pool_id", o.PoolID, "object_id", o.ID, "object_name", o.Name)
	if err := ctx.Err(); err != nil {
		return err
	}

	return xattr.Set(f.path(o.PoolID, o.ID), o)
}

// MoveObject moves an object file to a different pool directory
func (f *FS) MoveObject(ctx context.Context, o *hos.Object, destPoolID string) error {
	f.log.Debug("moving object", "src_pool_id", o.PoolID, "object_id", o.ID, "object_name", o.Name, "dst_pool_id", destPoolID)
	if err := ctx.Err(); err != nil {
		return err
	}

	destObjID := id.Gen(destPoolID, o.Name)
	// move the object
	if err := os.Rename(f.path(o.PoolID, o.ID), f.path(destPoolID, destObjID)); err != nil {
		return err
	}

	o2 := *o
	o2.PoolID = destPoolID
	o2.ID = destObjID

	return xattr.Set(f.path(o2.PoolID, o2.ID), &o2)
}

// DeleteUser removes a user file from the filesystem
func (f *FS) DeleteUser(ctx context.Context, uid string) error {
	f.log.Debug("deleting user", "user_id", uid)
	if err := ctx.Err(); err != nil {
		return err
	}

	up := filepath.Join(f.root, uid)
	f.log.Debug("removing file", "path", up)
	if err := os.Remove(up); err != nil {
		// we may need to handle special errors here??
		return err
	}

	return nil
}

// DeleteKey removes an encryption key file from the filesystem
func (f *FS) DeleteKey(ctx context.Context, kid string) error {
	f.log.Debug("deleting key", "key_id", kid)
	if err := ctx.Err(); err != nil {
		return err
	}

	kp := f.path(keyPath, kid)
	f.log.Debug("removing file", "path", kp)
	if err := os.Remove(kp); err != nil {
		return err
	}

	return nil
}

// DeletePool removes a pool directory from the filesystem
func (f *FS) DeletePool(ctx context.Context, pid string) error {
	f.log.Debug("deleting pool", "pool_id", pid)
	if err := ctx.Err(); err != nil {
		return err
	}

	pf := filepath.Join(f.root, pid)
	f.log.Debug("removing directory", "path", pf)
	if err := os.Remove(pf); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s %w", pid, hos.ErrNotExist)
		}
		if errors.Is(err, syscall.ENOTEMPTY) {
			return fmt.Errorf("%s %w", pid, hos.ErrNotEmpty)
		}
		return err
	}

	return nil
}

// DeleteObject removes an object file from the filesystem
func (f *FS) DeleteObject(ctx context.Context, pid, oid string) error {
	f.log.Debug("deleting object", "pool_id", pid, "object_id", oid)
	if err := ctx.Err(); err != nil {
		return err
	}

	op := filepath.Join(f.root, pid, oid)
	f.log.Debug("removing file", "path", op)
	if err := os.Remove(op); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s %w", pid, hos.ErrNotExist)
		}
		return err
	}

	return nil
}

// getDiskInfo retrieves filesystem statistics using statfs system call
func getDiskInfo(root string, log *slog.Logger) (*unix.Statfs_t, error) {
	log.Debug("getting statfs", "path", root)
	s := unix.Statfs_t{}
	if e := unix.Statfs(root, &s); e != nil {
		return nil, &os.PathError{Op: "stat", Path: root, Err: e}
	}

	return &s, nil
}

// GetDiskInfo returns filesystem usage statistics
func (f *FS) GetDiskInfo() (*hos.Statfs, error) {
	s, err := getDiskInfo(f.root, f.log)
	if err != nil {
		return nil, err
	}

	reservedBlocks := s.Bfree - s.Bavail
	total := uint64(s.Bsize) * (s.Blocks - reservedBlocks)
	free := uint64(s.Bsize) * s.Bavail

	if s.Bavail > s.Blocks {
		return nil, fmt.Errorf("free %d > total (%d), fs corruption at (%s)", free, total, f.root)
	}

	return &hos.Statfs{
		BlockSize: uint32(s.Bsize), Blocks: s.Blocks, BlocksFree: s.Bfree,
		BlocksAvailable: s.Bavail, Inodes: s.Files, InodesFree: s.Ffree,
	}, nil
}
