// SPDX-License-Identifier: MIT

package hosfs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brlbil/hos"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
)

func convErr(err error) error {
	if errors.Is(err, hos.ErrNotExist) {
		return fuse.ENOENT
	}
	return fuse.EIO
}

type inode struct {
	pool     hos.Pool
	object   hos.Object
	children map[string]fuseops.InodeID
	parentID fuseops.InodeID
	isDir    bool
}

func (i *inode) ID() fuseops.InodeID {
	if i.pool.ID != "" {
		return toInode(i.pool.ID)
	}
	if i.object.ID != "" {
		return toInode(i.object.ID)
	}
	return fuseops.RootInodeID
}

func (i *inode) PoolID() string {
	if i.pool.ID != "" {
		return i.pool.ID
	}
	if i.object.PoolID != "" {
		return i.object.PoolID
	}
	return ""
}

func (i *inode) Path() string {
	if i.object.Name != "" {
		return strings.TrimSuffix(i.object.Name, "/")
	}
	return ""
}

func (i *inode) Attributes() fuseops.InodeAttributes {
	if i.pool.ID != "" {
		return attributes(i.pool.Size, true, i.pool.ModifiedAt, i.pool.CreatedAt)
	}

	if i.object.ID != "" {
		return attributes(i.object.Size, i.isDir, i.object.ModifiedAt, i.object.CreatedAt)
	}

	return attributes(0, true, time.Time{}, time.Time{})
}

func sprintList[T any](tt ...T) string {
	ss := []string{}
	for _, t := range tt {
		ss = append(ss, fmt.Sprint(t))
	}
	return fmt.Sprintf("(%s)", strings.Join(ss, ","))
}
