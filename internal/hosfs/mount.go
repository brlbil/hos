// SPDX-License-Identifier: MIT

package hosfs

import (
	"context"
	"fmt"
	"io"
	llog "log"
	"log/slog"
	"os"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/logger"
	"github.com/brlbil/hos/pkg/client"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

const (
	errNotInCache = hos.ConstError("not exists in cache")
	errObjectNil  = hos.ConstError("inode's object is empty")
)

var log = slog.New(slog.DiscardHandler) // default log level is None

var (
	poolIDs = []string{}

	impUser          string
	fuseDebugLogging bool
)

// ConfigFunc is a configuration function for hosfs mount options
type ConfigFunc func() error

// SetLogging configures logging level for hosfs
func SetLogging(level string) ConfigFunc {
	return func() error {
		lev, err := logger.NewLevel(level)
		if err != nil {
			return err
		}

		var w io.Writer = os.Stderr
		opt := &slog.HandlerOptions{}
		if lev == logger.None {
			w = io.Discard
		} else {
			opt.Level = lev
		}

		log = slog.New(slog.NewTextHandler(w, opt))
		return nil
	}
}

// SetPoolID configures specific pool IDs to mount
func SetPoolID(ids ...string) ConfigFunc {
	return func() error {
		poolIDs = ids
		return nil
	}
}

// EnableFuseDebugLog enables FUSE debug logging
func EnableFuseDebugLog() error {
	fuseDebugLogging = true
	return nil
}

// SetImpersonatedUser configures user impersonation for hosfs
func SetImpersonatedUser(user string) func() error {
	return func() error {
		impUser = user
		return nil
	}
}

// Mount creates and mounts a HOS FUSE filesystem at the specified root directory
func Mount(ctx context.Context, root string, c *client.Client, conf ...ConfigFunc) error {
	for _, cf := range conf {
		if err := cf(); err != nil {
			return err
		}
	}

	log.Debug("stat root directory", "root dir", root)
	sr, err := os.Stat(root)
	if err != nil {
		return err
	} else if !sr.IsDir() {
		return fmt.Errorf("%s not a directory", root)
	}

	uid = uint32(os.Getuid())
	gid = uint32(os.Getgid())

	// certain this does not return error so we can ignore it
	_ = c.Reconfigure(client.PinContentServer)
	hfs, err := newHfs(c)
	if err != nil {
		return err
	}

	srv := fuseutil.NewFileSystemServer(hfs)

	opt := &fuse.MountConfig{FSName: "hosfs", ReadOnly: true}
	if fuseDebugLogging {
		opt.DebugLogger = llog.New(os.Stderr, "hosfs", 0)
	}
	mfs, err := fuse.Mount(root, srv, opt)
	if err != nil {
		return err
	}

	// Wait for it to be unmounted.
	if err = mfs.Join(ctx); err != nil {
		return err
	}

	return nil
}
