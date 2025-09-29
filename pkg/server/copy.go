// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/id"
	"github.com/minio/sio"
)

// copyLocal performs object copying within the same server
func (s *Server) copyLocal(ctx context.Context, r *http.Request) (*hos.Object, error) {
	dstPool := r.Header.Get(header.DestPool)
	newName := r.Header.Get(header.NewObjectName)

	user := getUser(ctx)
	if err := validate.ID(dstPool); dstPool != "" && err != nil {
		dstPool = id.Gen(user.ID, dstPool)
	}

	so := getObject(ctx)
	// same pool and not a new name copy not makes sense
	samePool := (dstPool == "" || dstPool == so.PoolID)
	if samePool && newName == "" {
		return nil, hos.ErrExist
	}

	var dpool *hos.Pool
	if samePool {
		dpool = getPool(ctx)
	}

	var err error
	if dpool == nil {
		dpool, err = db.Get[hos.Pool](ctx, s.db, dstPool)
		if err != nil {
			return nil, err
		}
	}

	if dpool.LinkedID != "" {
		dpool, err = db.Get[hos.Pool](ctx, s.db, dpool.LinkedID)
		if err != nil {
			return nil, err
		}
	}

	// check permissions
	if dpool.UserID != user.ID && !checkPermission(user.Name, write, dpool.Permissions, s.log.Logger) {
		return nil, hos.ErrInsufficientPermissions
	}

	encKey := r.Header.Get(header.EncryptionKey)
	var soEncConf *sio.Config
	if !samePool && so.Encrypted {
		s.log.Debug("getting encryption key", "func", "copyLocal")
		mkey, err := getMutatedKey(ctx, user.ID, encKey, so, s.db)
		if err != nil {
			if errors.Is(err, hos.ErrNotExist) {
				err = errors.Join(err, hos.ErrDecryption)
			}

			return nil, fmt.Errorf("getting encryption key failed %w", err)
		}
		ec := defEncConf
		ec.Key = mkey
		soEncConf = &ec
	}

	rsc, err := s.fs.ReadObject(ctx, so, soEncConf)
	if err != nil {
		return nil, err
	}

	// copy object
	o := *so
	// modify dest object
	ct := time.Now()
	if newName != "" {
		o.Name = newName
	}
	o.ID = id.Gen(dpool.ID, o.Name)
	o.UserID = user.ID
	o.PoolID = dpool.ID
	o.ReplicaCount = dpool.ReplicaCount
	o.Encrypted = dpool.Encrypted
	o.CreatedAt = ct
	o.ModifiedAt = ct

	var dstEncConf *sio.Config
	if !samePool && dpool.Encrypted {
		s.log.Debug("getting encryption key", "func", "copyLocal")
		mkey, err := getMutatedKey(ctx, user.ID, encKey, &o, s.db)
		if err != nil {
			if errors.Is(err, hos.ErrNotExist) {
				err = errors.Join(err, hos.ErrDecryption)
			}

			return nil, fmt.Errorf("getting encryption key failed %w", err)
		}
		ec := defEncConf
		ec.Key = mkey
		dstEncConf = &ec
	}

	if err := s.fs.CreateObject(ctx, &o, rsc, dstEncConf); err != nil {
		return nil, fmt.Errorf("creating object failed %w", err)
	}

	if err := db.Create(ctx, s.db, &o); err != nil {
		return nil, fmt.Errorf("creating object failed %w", err)
	}

	// update the pool count and size
	dpool.ObjectCount++
	dpool.Size += o.Size
	dpool.Hash = calHash(dpool.Hash, "+", o.Hash)
	dpool.ModifiedAt = ct

	if err := s.fs.UpdatePool(ctx, dpool); err != nil {
		return nil, fmt.Errorf("updating pool failed %w", err)
	}

	if err := db.Update(ctx, s.db, dpool); err != nil {
		return nil, fmt.Errorf("updating pool failed %w", err)
	}

	return &o, nil
}

// copyRemote performs object copying to a remote server
func (s *Server) copyRemote(ctx context.Context, r *http.Request) (*http.Response, error) {
	if r.ContentLength > 1024 {
		return nil, fmt.Errorf("ca key is larger than expected %w", hos.ErrContentTooLarge)
	}

	var caCert []byte
	var err error
	if r.ContentLength > 0 {
		defer r.Body.Close()
		caCert, err = io.ReadAll(io.LimitReader(r.Body, r.ContentLength))
		if err != nil {
			return nil, fmt.Errorf("reading ca cert failed %w", err)
		}
	}
	if len(caCert) == 0 {
		return nil, fmt.Errorf("CA is empty %w", hos.ErrBadRequest)
	}

	dstServer := r.Header.Get(header.DestServer)
	if dstServer == s.srv.Addr {
		return nil, fmt.Errorf("same server copy vi http is %w", hos.ErrNotAllowed)
	}

	dstToken := r.Header.Get(header.DestToken)
	if dstToken == "" {
		return nil, fmt.Errorf("destination token is empty %w", hos.ErrBadRequest)
	}

	s.log.Debug("parsing token")
	dsu, tokenStatus := parseToken(dstToken)
	if tokenStatus != tokenOK {
		return nil, fmt.Errorf("destination token is melformed %w", hos.ErrBadRequest)
	}
	dstUser := dsu[:8]

	dstPool := r.Header.Get(header.DestPool)
	if dstPool == "" {
		dstPool = getPool(ctx).Name
	}
	if err := validate.ID(dstPool); err != nil {
		dstPool = id.Gen(dstUser, dstPool)
	}

	so := getObject(ctx)
	obj := &hos.Object{
		PoolID:      dstPool,
		Name:        so.Name,
		ContentType: so.ContentType,
		Size:        so.Size,
	}

	newName := r.Header.Get(header.NewObjectName)
	if newName != "" {
		obj.Name = newName
	}

	clt, err := newClient(dstServer, string(caCert))
	if err != nil {
		return nil, fmt.Errorf("initializing a new client failed %w", err)
	}

	encKey := r.Header.Get(header.EncryptionKey)
	var ecnf *sio.Config
	// if user provides enckey then decrypty the object
	if so.Encrypted && encKey != "" {
		s.log.Debug("getting encryption key", "func", "copyRemote")
		user := getUser(ctx)
		mkey, err := getMutatedKey(ctx, user.ID, encKey, so, s.db)
		if err != nil {
			if errors.Is(err, hos.ErrNotExist) {
				err = errors.Join(err, hos.ErrDecryption)
			}

			return nil, fmt.Errorf("getting encryption key failed %w", err)
		}
		ec := defEncConf
		ec.Key = mkey
		ecnf = &ec
	}

	newEncKey := r.Header.Get(header.EncryptionNewKey)
	cltFns := []clientFunc{uploadEncObject(s.fs, so, ecnf), setHeader("Authorization", prefix+dstToken)}
	if newEncKey != "" {
		cltFns = append(cltFns, setHeader(header.EncryptionKey, newEncKey))
	}

	// we do not want to decrypty the object just send it as it is
	if so.Encrypted && encKey == "" && newEncKey == "" {
		cltFns = append(cltFns, setHeader(header.OriginalHash, so.Hash))
	}
	cltFns = append(cltFns, marshalHeader(obj))

	return clt.do(ctx, put, path.Join(constant.APIPrefix, dstPool), cltFns...)
}
