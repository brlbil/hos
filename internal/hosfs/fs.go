// SPDX-License-Identifier: MIT

// Package hosfs provides a FUSE filesystem implementation for HOS.
// It mounts HOS pools and objects as a read-only filesystem using FUSE.
package hosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/cache"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
	"github.com/dustin/go-humanize"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

var (
	uid uint32
	gid uint32
)

type hfs struct {
	fuseutil.NotImplementedFileSystem
	userID   string
	cacheMap *sync.Map
	poolIDs  []string
	client   *client.Client
	reqOpts  []client.Options
}

func newHfs(clt *client.Client) (*hfs, error) {
	log.Debug("New Hos FileSystem", "pools", poolIDs, "pools length", len(poolIDs))

	fs := &hfs{client: clt, userID: id.Gen(clt.User()), cacheMap: &sync.Map{}, poolIDs: poolIDs}
	if impUser != "" {
		fs.userID = id.Gen(impUser)
		fs.reqOpts = []client.Options{client.OnBehalf(impUser)}
	}
	rootNode := inode{isDir: true, children: map[string]fuseops.InodeID{}}

	if len(poolIDs) > 0 {
		// normally not all pools are required, checking if all of them exists
		pools, err := clt.ListPools(context.Background(), append(fs.reqOpts, client.FilterByField("ID", poolIDs...))...)
		if err != nil {
			return nil, err
		}

		poolIDList := []string{}
		for _, pool := range pools {
			poolIDList = append(poolIDList, pool.ID)
		}

		for _, poolID := range poolIDs {
			if !slices.Contains(poolIDList, poolID) {
				return nil, fmt.Errorf("pool id %s not found", poolID)
			}
		}

		if len(poolIDs) == 1 {
			rootNode.pool = pools[0]
		}
	}

	// MAYBE children can be set here

	cache.Set[fuseops.InodeID, inode](fs.cacheMap, fuseops.RootInodeID, rootNode, 0)

	return fs, nil
}

func toInode(id string) fuseops.InodeID {
	inode, err := strconv.ParseInt(id, 16, 64)
	if err != nil {
		panic(err)
	}
	return fuseops.InodeID(inode + 1)
}

func attributes(size int64, isDirectory bool, modTime, createTime time.Time) fuseops.InodeAttributes {
	if size == 0 {
		size = 64
	}

	var mode fs.FileMode = 0o440
	if isDirectory {
		mode = 0o550 | os.ModeDir
	}

	return fuseops.InodeAttributes{
		Size:   uint64(size),
		Nlink:  1,
		Mode:   mode,
		Atime:  modTime,
		Mtime:  modTime,
		Ctime:  createTime,
		Crtime: createTime,
		Uid:    uid,
		Gid:    gid,
	}
}

func (fs *hfs) getInodeFromCache(ctx context.Context, parentID fuseops.InodeID, name string) (*inode, *inode, error) {
	logger := log.With("func", "getInodeFromCache")
	logger.Debug("getting parent inode", "parent_inode", parentID, "name", name)
	parent, ok := cache.Get[fuseops.InodeID, inode](fs.cacheMap, parentID)
	if !ok {
		logger.Debug("getting parent inode failed", "error", errNotInCache, "parent_inode", parentID)
		return nil, nil, errNotInCache
	}

	childID, ok := parent.children[name]
	if !ok {
		logger.Debug("getting child failed", "error", "not exist in children", "name", name)
		return &parent, nil, errNotInCache
	}

	child, ok := cache.Get[fuseops.InodeID, inode](fs.cacheMap, childID)
	if !ok {
		logger.Debug("getting child failed", "error", errNotInCache, "child_inode", childID)
		return &parent, nil, errNotInCache
	}

	logger.Debug("got inode from cache", "inode", childID, "name", name)

	return &parent, &child, nil
}

func (fs *hfs) getInode(ctx context.Context, parentID fuseops.InodeID, name string) (*inode, error) {
	// sanitize the name
	name = strings.ReplaceAll(name, "\\", "")

	logger := log.With("func", "getInode")
	logger.Debug("getting inode", "parent inode", parentID, "name", name)

	parent, child, err := fs.getInodeFromCache(ctx, parentID, name)
	if err == nil {
		return child, nil
	}
	if parent == nil {
		return nil, fuse.ENOENT
	}

	// root inode and it is not a pool
	if parentInodeID := parent.ID(); parentInodeID == fuseops.RootInodeID && parent.pool.ID == "" {
		poolID := id.Gen(fs.userID, name)
		if len(fs.poolIDs) > 1 && !slices.Contains(fs.poolIDs, poolID) {
			logger.Error("checking pool id failed", "error", errors.New("id is not in pool id list"), "pool_name", name, "pool_id", poolID)
			return nil, fuse.ENOENT
		}

		logger.Debug("getting pool", "pool_id", poolID)
		pool, err := fs.client.GetPool(ctx, poolID, append(fs.reqOpts, client.IgnoreErrors(hos.ErrNotExist))...)
		if err != nil {
			logger.Error("getting pool failed", "error", err, "pool_id", poolID, "name", name)
			return nil, convErr(err)
		}

		inodeID := toInode(poolID)
		inodeEntry := inode{pool: *pool, parentID: parentID, children: map[string]fuseops.InodeID{}, isDir: true}
		logger.Debug("set inode to cache", "inode", inodeID, "pool_id", poolID, "pool_name", pool.Name)
		cache.Set(fs.cacheMap, inodeID, inodeEntry, 0)

		return &inodeEntry, nil
	}

	poolID := parent.PoolID()
	prefix := parent.Path()
	pathName := path.Join(prefix, name)
	logger.Debug("listing objects", "pool_id", poolID, "prefix", prefix)
	objects, err := fs.client.ListObjects(ctx, poolID,
		append(fs.reqOpts,
			client.IgnoreErrors(hos.ErrNotExist),
			filter.NamePrefix(prefix),
			client.ObjectDirectoryListing(prefix),
			client.FilterByField("Name", pathName, pathName+"/"),
		)...,
	)
	if err != nil {
		logger.Error("listing objects failed", "error", err, "pool_id", poolID, "name", name, "prefix", prefix)
		return nil, convErr(err)
	}
	if len(objects) == 0 {
		logger.Error("listing objects failed", "error", errors.New("no objects returned"), "pool_id", poolID, "name", name, "prefix", prefix)
		return nil, fuse.ENOENT
	}

	object := objects[0]
	var isDirectory bool
	if strings.HasSuffix(object.Name, "/") {
		isDirectory = true
	}

	inodeID := toInode(object.ID)
	inodeEntry := inode{object: object, parentID: parentID, children: map[string]fuseops.InodeID{}, isDir: isDirectory}
	logger.Debug("set inode to cache", "inode", inodeID, "pool_id", object.PoolID, "object_id", object.ID, "object_name", object.Name)
	cache.Set(fs.cacheMap, inodeID, inodeEntry, 0)

	return &inodeEntry, nil
}

func (fs *hfs) StatFS(ctx context.Context, op *fuseops.StatFSOp) error {
	l := log.With("func", "StatFS")
	l.Debug("getting ServerInfo from cache")
	srvInfo, ok := cache.Get[string, hos.ServerInfo](fs.cacheMap, "statfs")
	if ok {
		log.Debug("red ServerInfo from cache", "func", "StatFS", "free_disk", humanize.Bytes(srvInfo.FreeDisk()))
		op.BlockSize = srvInfo.BlockSize
		op.Blocks = srvInfo.Blocks
		op.BlocksFree = srvInfo.BlocksFree
		op.BlocksAvailable = srvInfo.BlocksAvailable
		op.Inodes = srvInfo.Inodes
		op.InodesFree = srvInfo.InodesFree
		return nil
	}

	l.Debug("getting ServerInfo")
	sif, err := fs.client.GetServerInfo(ctx, append(fs.reqOpts, client.IgnoreErrors(hos.ErrConnectionFailure))...)
	if err != nil {
		l.Error("getting server info failed", "error", err)
		return fuse.EIO
	}

	si := hos.ServerInfo{}
	for _, s := range sif {
		if si.BlockSize == 0 {
			si.BlockSize = s.BlockSize
		}
		if si.BlockSize != s.BlockSize {
			s.Blocks = (s.Blocks * uint64(s.BlockSize)) / uint64(si.BlockSize)
			s.BlocksFree = (s.BlocksFree * uint64(s.BlockSize)) / uint64(si.BlockSize)
			s.BlocksAvailable = (s.BlocksAvailable * uint64(s.BlockSize)) / uint64(si.BlockSize)
		}
		si.Blocks += s.Blocks
		si.BlocksAvailable += s.BlocksAvailable
		si.BlocksFree += s.BlocksFree
		si.Inodes += s.Inodes
		si.InodesFree += s.InodesFree
	}

	l.Debug("set ServerInfo to cache", "free_disk", humanize.Bytes(srvInfo.FreeDisk()))
	cache.Set(fs.cacheMap, "statfs", si, time.Minute*20)

	op.BlockSize = si.BlockSize
	op.Blocks = si.Blocks
	op.BlocksFree = si.BlocksFree
	op.BlocksAvailable = si.BlocksAvailable
	op.Inodes = si.Inodes
	op.InodesFree = si.InodesFree

	return nil
}

func (fs *hfs) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) error {
	log.Debug("getting inode", "func", "LookUpInode", "parent", op.Parent, "name", op.Name)
	entry, err := fs.getInode(ctx, op.Parent, op.Name)
	if err != nil {
		return err
	}

	outputEntry := &op.Entry
	outputEntry.Child = entry.ID()
	outputEntry.Attributes = entry.Attributes()
	return nil
}

func (fs *hfs) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	l := log.With("func", "GetInodeAttributes")
	l.Debug("getting inode from cache", "inode", op.Inode)
	entry, found := cache.Get[fuseops.InodeID, inode](fs.cacheMap, op.Inode)
	if !found {
		l.Error("getting inode failed", "error", errNotInCache, "inode", op.Inode)
		return fuse.ENOENT
	}
	op.Attributes = entry.Attributes()
	return nil
}

func (fs *hfs) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) error {
	return nil
}

func (fs *hfs) listChildren(ctx context.Context, in *inode) ([]fuseutil.Dirent, error) {
	l := log.With("func", "ListChildren")
	var dirents []fuseutil.Dirent
	inID := in.ID()

	// if root inode is not a pool
	if inID == fuseops.RootInodeID && in.pool.ID == "" {
		l.Debug("listing inode children", "inode", inID)
		opts := append(fs.reqOpts, client.IgnoreErrors(hos.ErrNotExist))
		if len(fs.poolIDs) > 0 {
			opts = append(opts, client.FilterByField("ID", fs.poolIDs...))
		}

		l.Debug("listing pools", "options", sprintList(opts))
		pools, err := fs.client.ListPools(ctx, opts...)
		if err != nil {
			l.Error("listing pools failed", "error", err)
			return nil, fuse.EIO
		}

		for i, p := range pools {
			nin := inode{pool: p, parentID: inID, children: map[string]fuseops.InodeID{}, isDir: true}
			ninID := nin.ID()

			l.Debug("adding inode to cache", "inode", ninID, "name", p.Name, "pool_id", p.ID)
			cache.Set(fs.cacheMap, ninID, nin, 0)
			in.children[p.Name] = ninID

			de := fuseutil.Dirent{
				Offset: fuseops.DirOffset(i + 1),
				Inode:  ninID,
				Name:   p.Name,
				Type:   fuseutil.DT_Directory,
			}

			dirents = append(dirents, de)
		}

		return dirents, nil
	}

	poolID := in.PoolID()
	l.Debug("listing inode children", "inode", inID, "pool_id", poolID, "pool_name", in.pool.Name,
		"object_name", in.object.Name, "object_id", in.object.ID)
	inPath := in.Path()

	l.Debug("listing objects", "pool_id", poolID, "prefix", inPath)
	objs, err := fs.client.ListObjects(ctx, poolID,
		append(fs.reqOpts,
			client.IgnoreErrors(hos.ErrNotExist),
			filter.NamePrefix(inPath),
			client.ObjectDirectoryListing(inPath),
		)...,
	)
	if err != nil {
		l.Error("listing objects", "error", err, "pool id", poolID, "prefix", inPath)
		return nil, fuse.EIO
	}

	for i, o := range objs {
		t := fuseutil.DT_Directory
		isDir := true
		name := path.Base(o.Name)
		if !strings.HasSuffix(o.Name, "/") {
			t = fuseutil.DT_File
			isDir = false
		}

		nin := inode{object: o, parentID: inID, children: map[string]fuseops.InodeID{}, isDir: isDir}
		ninID := nin.ID()

		l.Debug("adding inode to cache", "inode", ninID, "name",
			name, "object_id", o.ID, "object_name", o.Name, "dir", isDir)

		cache.Set(fs.cacheMap, ninID, nin, 0)
		in.children[name] = ninID

		de := fuseutil.Dirent{
			Offset: fuseops.DirOffset(i + 1),
			Inode:  toInode(o.ID),
			Name:   name,
			Type:   t,
		}

		dirents = append(dirents, de)
	}

	return dirents, nil
}

func (fs *hfs) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	l := log.With("func", "ReadDir")
	l.Debug("reading directory", "inode", op.Inode)
	entry, found := cache.Get[fuseops.InodeID, inode](fs.cacheMap, op.Inode)
	if !found {
		l.Error("getting inode failed", "error", errNotInCache, "inode", op.Inode)
		return fuse.ENOENT
	}

	// MAYBE check last time children updated and decide accordingly
	// clean child cache
	for name, childID := range entry.children {
		l.Debug("deleting inode from cache", "inode", childID, "name", name)
		cache.Delete[fuseops.InodeID](fs.cacheMap, childID)
	}

	children, err := fs.listChildren(ctx, &entry)
	if err != nil {
		return fuse.EIO
	}

	if op.Offset > fuseops.DirOffset(len(children)) {
		return fuse.EIO
	}

	children = children[op.Offset:]

	for _, child := range children {
		bytesWritten := fuseutil.WriteDirent(op.Dst[op.BytesRead:], child)
		if bytesWritten == 0 {
			break
		}
		op.BytesRead += bytesWritten
	}

	return nil
}

func (fs *hfs) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) error {
	return nil
}

func (fs *hfs) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) error {
	l := log.With("func", "ReadFile")
	entry, found := cache.Get[fuseops.InodeID, inode](fs.cacheMap, op.Inode)
	if !found {
		l.Debug("getting inode failed", "inode", op.Inode, "offset", op.Offset, "size", op.Size)
		return fuse.ENOENT
	}

	if entry.object.ID == "" {
		l.Error("getting inode object failed", "error", errObjectNil)
		return errObjectNil
	}

	obj, err := fs.client.GetContent(ctx, entry.object.PoolID, entry.object.ID,
		append(fs.reqOpts,
			client.IgnoreErrors(hos.ErrNotExist),
			client.Headers(map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", op.Offset, op.Offset+op.Size-1)}),
		)...,
	)
	if err != nil {
		l.Error("getting object content failed", "error", err, "pool_id", entry.object.PoolID, "object_id", entry.object.ID,
			"object_name", entry.object.Name)
		return convErr(err)
	}

	buf, err := io.ReadAll(io.LimitReader(obj, obj.Size))
	if err != nil {
		l.Error("reading object content failed", "error", err, "pool_id", obj.PoolID, "object_id", obj.ID,
			"object_name", obj.Name, "object_size", obj.Size)
		return convErr(err)
	}
	op.BytesRead = copy(op.Dst, buf)
	return nil
}

func (fs *hfs) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) error {
	return nil
}

func (fs *hfs) GetXattr(ctx context.Context, op *fuseops.GetXattrOp) error {
	return nil
}

func (fs *hfs) ListXattr(ctx context.Context, op *fuseops.ListXattrOp) error {
	return nil
}

func (fs *hfs) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) error {
	return nil
}

func (fs *hfs) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) error {
	return nil
}

func (fs *hfs) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) error {
	return nil
}
