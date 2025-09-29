// SPDX-License-Identifier: MIT

package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/pkg/id"
	"github.com/minio/sio"
)

var defEncConf = sio.Config{
	MinVersion:   sio.Version20,
	MaxVersion:   sio.Version20,
	CipherSuites: []byte{sio.CHACHA20_POLY1305, sio.AES_256_GCM},
}

// listObjects handles GET /api/v1/{pid} - returns objects in the specified pool
func (s *Server) listObjects(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "ListObjects")
	ctx := r.Context()
	p := getPool(ctx)

	s.log.Debug("parsing HeaderOptions from header", "func", "ListObjects")
	opt, err := header.Parse[filter.Headers](r.Header)
	if err != nil {
		httpError(w, s.log.Logger, "parsing HeaderOptions failed", err, "func", "ListObjects")
		return
	}

	objs, err := db.List[hos.Object](ctx, s.db, p.ID, queryFuncs(opt)...)
	if err != nil {
		httpError(w, s.log.Logger, "listing objects failed", err, "func", "ListObjects")
		return
	}

	if objs == nil {
		objs = []hos.Object{}
	}

	if ol := len(objs); len(opt.Range) == 2 && ol != opt.Range[1] {
		opt.Range[1] = ol
	}

	writeHeaders(w, opt)
	writeJSON(w, objs)
}

// getObject handles HEAD /api/v1/{pid}/{oid} - returns object metadata
func (s *Server) getObject(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "GetObject")
	o := getObject(r.Context())

	writeHeaders(w, o)
	w.WriteHeader(http.StatusNoContent)
}

// getObjectContent handles GET /api/v1/{pid}/{oid} - downloads object data
func (s *Server) getObjectContent(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "GetObjectContent")
	ctx := r.Context()
	obj := getObject(ctx)
	user := getUser(ctx)

	var encryptionConfig *sio.Config
	if obj.Encrypted {
		s.log.Debug("getting encryption key", "func", "GetObjectContent")
		mutatedKey, err := getMutatedKey(ctx, user.ID, r.Header.Get(header.EncryptionKey), obj, s.db)
		if err != nil {
			if errors.Is(err, hos.ErrNotExist) {
				err = fmt.Errorf("%s %w", err, hos.ErrDecryption) //nolint:errorlint
			}
			httpError(w, s.log.Logger, "getting encryption key failed", err, "func", "GetObjectContent")
			return
		}
		encConfig := defEncConf
		encConfig.Key = mutatedKey
		encryptionConfig = &encConfig
	}

	rsc, err := s.fs.ReadObject(ctx, obj, encryptionConfig)
	if err != nil {
		httpError(w, s.log.Logger, "getting object content failed", err, "func", "GetObjectContent")
		return
	}
	defer rsc.Close()

	o := getObject(ctx)

	writeHeaders(w, o)
	// set file name as header
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, path.Base(o.Name)))
	http.ServeContent(w, r, o.Name, o.ModifiedAt, rsc)
}

// createObject handles PUT /api/v1/{pid} - uploads a new object to the pool
func (s *Server) createObject(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "CreateObject")
	s.log.Debug("parsing object from request", "func", "CreateObject")
	o, err := fromRequest[hos.Object](r)
	if err != nil {
		httpError(w, s.log.Logger, "parsing object from request failed", err, "func", "CreateObject")
		return
	}

	ctx := r.Context()
	u := getUser(ctx)
	pool := getPool(ctx)
	id := id.Gen(pool.ID, o.Name)

	ct := time.Now()
	o.ID = id
	o.UserID = u.ID
	o.PoolID = pool.ID
	o.ReplicaCount = pool.ReplicaCount
	o.Encrypted = pool.Encrypted
	o.CreatedAt = ct
	o.ModifiedAt = ct

	// let's close the body when we are finished
	defer r.Body.Close()

	user := getUser(ctx)

	var encryptionConfig *sio.Config
	if o.Encrypted {
		s.log.Debug("getting encryption key", "func", "CreateObject")
		mutatedKey, err := getMutatedKey(ctx, user.ID, r.Header.Get(header.EncryptionKey), o, s.db)
		if err != nil {
			httpError(w, s.log.Logger, "getting encryption key failed", err, "func", "GetObjectContent")
			return
		}
		encConfig := defEncConf
		encConfig.Key = mutatedKey
		encryptionConfig = &encConfig
	}

	// object size is unknown, needs to be set by fs.CreateObject
	if r.Header.Get(header.SizeUnknown) != "" {
		o.Size = -123
	}

	if err := s.fs.CreateObject(ctx, o, r.Body, encryptionConfig); err != nil {
		httpError(w, s.log.Logger, "creating object failed", err, "func", "CreateObject")
		return
	}

	if err := db.Create(ctx, s.db, o); err != nil {
		httpError(w, s.log.Logger, "creating object failed", err, "func", "CreateObject")
		return
	}

	// update the pool count and size
	pool.ObjectCount++
	pool.Size += o.Size
	pool.Hash = calHash(pool.Hash, "+", o.Hash)
	pool.ModifiedAt = ct

	if err := s.fs.UpdatePool(ctx, pool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", pool.ID, "func", "CreateObject")
		return
	}

	if err := db.Update(ctx, s.db, pool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", pool.ID, "func", "CreateObject")
		return
	}

	writeHeaders(w, o)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) editObject(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "EditObject")
	s.log.Debug("parsing object from request", "func", "EditObject")
	object, err := fromRequest[hos.Object](r)
	if err != nil {
		httpError(w, s.log.Logger, "parsing object from request failed", err, "func", "EditObject")
		return
	}

	ctx := r.Context()
	o := getObject(ctx)
	// modify Labels
	s.log.Debug("merging labels", "func", "EditObject")
	o.Labels, err = mergeMaps(o.Labels, object.Labels)
	if err != nil {
		httpError(w, s.log.Logger, "merging labels failed", err, "func", "EditObject")
		return
	}
	if object.ContentType != "" {
		o.ContentType = object.ContentType
	}

	// set modtime
	o.ModifiedAt = time.Now()

	if err := s.fs.UpdateObject(ctx, o); err != nil {
		httpError(w, s.log.Logger, "updating object failed", err, "object_id", o.ID, "func", "EditObject")
		return
	}

	if err := db.Update(ctx, s.db, o); err != nil {
		httpError(w, s.log.Logger, "updating object failed", err, "object_id", o.ID, "func", "EditObject")
		return
	}

	writeHeaders(w, o)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) moveObject(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "MoveObject")
	ctx := r.Context()

	// compare destination pool to source pool
	spool := getPool(ctx)
	dpool := getDestPool(ctx)
	if spool.UserID != dpool.UserID {
		httpError(w, s.log.Logger, "owner comparison failed", fmt.Errorf("owners must match, %w", hos.ErrNotAllowed), "func", "MoveObject")
		return
	}
	if spool.ReplicaCount != dpool.ReplicaCount {
		httpError(w, s.log.Logger, "comparing pools failed", fmt.Errorf("replication count %w", hos.ErrNotEqual), "func", "MoveObject")
		return
	}
	if ea := spool.Encrypted; ea != dpool.Encrypted {
		httpError(w, s.log.Logger, "comparing pools failed", fmt.Errorf("encryption attribute %w", hos.ErrNotEqual), "func", "MoveObject")
		return
	}

	o := getObject(ctx)

	// update object
	modTime := time.Now()
	o.ModifiedAt = modTime

	if name := r.Header.Get(header.NewObjectName); name != "" {
		o.Name = name
	}

	if err := s.fs.MoveObject(ctx, o, dpool.ID); err != nil {
		httpError(w, s.log.Logger, "moving object failed", err, "object_id", o.ID, "func", "MoveObject")
		return
	}

	// delete object and create again in DB
	if err := db.Delete[hos.Object, hos.Object](ctx, s.db, o.ID); err != nil {
		httpError(w, s.log.Logger, "moving object failed", err, "object_id", o.ID, "func", "MoveObject")
		return
	}

	// update poolID and id
	o.PoolID = dpool.ID
	o.ID = id.Gen(dpool.ID, o.Name)

	if err := db.Create(ctx, s.db, o); err != nil {
		httpError(w, s.log.Logger, "moving object failed", err, "new_object_id", o.ID, "func", "MoveObject")
		return
	}

	if dpool.ID == spool.ID {
		writeHeaders(w, o)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// update the source pool count and size
	spool.ObjectCount--
	spool.Size -= o.Size
	spool.Hash = calHash(spool.Hash, "-", o.Hash)
	spool.ModifiedAt = modTime

	if err := s.fs.UpdatePool(ctx, spool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", spool.ID, "func", "MoveObject")
		return
	}

	if err := db.Update(ctx, s.db, spool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", spool.ID, "func", "MoveObject")
		return
	}

	// update the destination pool count and size
	dpool.ObjectCount++
	dpool.Size += o.Size
	dpool.Hash = calHash(dpool.Hash, "+", o.Hash)
	dpool.ModifiedAt = modTime

	if err := s.fs.UpdatePool(ctx, dpool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", spool.ID, "func", "MoveObject")
		return
	}

	if err := db.Update(ctx, s.db, dpool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", spool.ID, "func", "MoveObject")
		return
	}

	writeHeaders(w, o)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) copyObject(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "CopyObject")
	ctx := r.Context()

	dstServer := r.Header.Get(header.DestServer)
	// same server
	if dstServer == "" {
		obj, err := s.copyLocal(ctx, r)
		if err != nil {
			httpError(w, s.log.Logger, "", err, "func", "CopyObject")
		}

		writeHeaders(w, obj)
		w.WriteHeader(http.StatusCreated)

		return
	}

	resp, err := s.copyRemote(ctx, r)
	if err != nil {
		httpError(w, s.log.Logger, "copy to remote server failed", err)
		return
	}

	if resp.StatusCode >= 400 {
		w.WriteHeader(resp.StatusCode)
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			s.log.Error("reading response body failed", "error", err)
		}
		fmt.Fprint(w, string(b))
	}
	if resp.StatusCode < 300 {
		// copy headers
		for k, v := range resp.Header {
			if len(v) > 0 {
				w.Header().Set(k, v[0])
			}
		}
		w.WriteHeader(resp.StatusCode)
	}
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "DeleteObject")
	ctx := r.Context()
	pool := getPool(ctx)
	obj := getObject(ctx)

	if err := s.fs.DeleteObject(ctx, pool.ID, obj.ID); err != nil {
		httpError(w, s.log.Logger, "deleting object failed", err, "object_id", obj.ID, "func", "DeleteObject")
		return
	}

	if err := db.Delete[hos.Object, hos.Object](ctx, s.db, obj.ID); err != nil {
		httpError(w, s.log.Logger, "deleting object failed", err, "object_id", obj.ID, "func", "DeleteObject")
		return
	}

	// update the pool count and size
	pool.ObjectCount--
	pool.Size -= obj.Size
	pool.Hash = calHash(pool.Hash, "-", obj.Hash)
	pool.ModifiedAt = time.Now()

	if err := s.fs.UpdatePool(ctx, pool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", pool.ID, "func", "DeleteObject")
		return
	}

	if err := db.Update(ctx, s.db, pool); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", pool.ID, "func", "DeleteObject")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
