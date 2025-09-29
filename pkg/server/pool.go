// SPDX-License-Identifier: MIT

package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/pkg/id"
	"github.com/go-chi/chi/v5"
)

// listPools handles GET /api/v1 - returns pools accessible to the user
func (s *Server) listPools(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "ListPools")
	ctx := r.Context()
	u := getUser(ctx)

	s.log.Debug("parsing HeaderOptions from header", "func", "ListPools")
	opt, err := header.Parse[filter.Headers](r.Header)
	if err != nil {
		httpError(w, s.log.Logger, "parsing HeaderOptions failed", err, "func", "ListPools")
		return
	}

	pools, err := db.List[hos.Pool](ctx, s.db, u.ID, queryFuncs(opt)...)
	if err != nil {
		httpError(w, s.log.Logger, "listing pools failed", err, "func", "ListPools")
		return
	}

	if pools == nil {
		pools = []hos.Pool{}
	}

	if pl := len(pools); len(opt.Range) == 2 && pl != opt.Range[1] {
		opt.Range[1] = pl
	}

	writeHeaders(w, opt)
	writeJSON(w, pools)
}

// getPool handles HEAD /api/v1/{pid} - returns pool metadata
func (s *Server) getPool(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "GetPool")
	writeHeaders(w, getPool(r.Context()))
	w.WriteHeader(http.StatusNoContent)
}

// createPool handles PUT /api/v1 - creates a new storage pool
func (s *Server) createPool(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "CreatePool")
	s.log.Debug("parsing pool from request", "func", "CreatePool")
	p, err := fromRequest[hos.Pool](r)
	if err != nil {
		httpError(w, s.log.Logger, "parsing pool from request failed", err, "func", "CreatePool")
		return
	}

	ctx := r.Context()
	u := getUser(ctx)
	// assign values
	p.UserID = u.ID
	ct := time.Now()
	p.CreatedAt = ct
	p.ModifiedAt = ct
	p.ID = id.Gen(u.ID, p.Name)
	if p.LinkedID == "" && p.ReplicaCount == 0 {
		p.ReplicaCount = 1
	}

	if err := s.fs.CreatePool(ctx, p); err != nil {
		httpError(w, s.log.Logger, "creating pool failed", err, "pool_id", p.ID, "func", "CreatePool")
		return
	}

	if err := db.Create(ctx, s.db, p); err != nil {
		httpError(w, s.log.Logger, "creating pool failed", err, "pool_id", p.ID, "func", "CreatePool")
		return
	}

	writeHeaders(w, p)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) editPool(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "EditPool")
	s.log.Debug("parsing pool from request", "func", "EditPool")
	pr, err := fromRequest[hos.Pool](r)
	if err != nil {
		httpError(w, s.log.Logger, "parsing pool from request failed", err, "func", "EditPool")
		return
	}

	ctx := r.Context()
	p := getPool(ctx)

	// it is not possible to edit attributes
	// if an existing attribute is to be changed return an error
	s.log.Debug("checking attributes", "func", "EditPool")
	// set Attributes if it is empty
	if p.Attributes == nil && len(pr.Attributes) > 0 {
		p.Attributes = map[string]string{}
	}
	for k, v := range pr.Attributes {
		if _, ok := p.Attributes[k]; ok {
			httpError(w, s.log.Logger, "checking attributes failed",
				fmt.Errorf("attribute %s is already exist %w", k, hos.ErrBadRequest), "func", "EditPool")
			return
		}
		p.Attributes[k] = v
	}

	// change the values
	s.log.Debug("merging labels", "func", "EditPool")
	p.Labels, err = mergeMaps(p.Labels, pr.Labels)
	if err != nil {
		httpError(w, s.log.Logger, "merging labels failed", err, "func", "EditPool")
		return
	}
	s.log.Debug("merging permissions", "func", "EditPool")
	p.Permissions, err = mergeMaps(p.Permissions, pr.Permissions)
	if err != nil {
		httpError(w, s.log.Logger, "merging permissions failed", err, "func", "EditPool")
		return
	}

	p.ModifiedAt = time.Now()
	if err := s.fs.UpdatePool(ctx, p); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", p.ID, "func", "EditPool")
		return
	}

	if err := db.Update(ctx, s.db, p); err != nil {
		httpError(w, s.log.Logger, "updating pool failed", err, "pool_id", p.ID, "func", "EditPool")
		return
	}

	writeHeaders(w, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deletePool(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "DeletePool")
	ctx := r.Context()

	pid := chi.URLParam(r, "pid")
	if err := s.fs.DeletePool(ctx, pid); err != nil {
		httpError(w, s.log.Logger, "deleting pool failed", err, "pool_id", pid, "func", "DeletePool")
		return
	}

	if err := db.Delete[hos.Pool, hos.Object](ctx, s.db, pid); err != nil {
		httpError(w, s.log.Logger, "deleting pool failed", err, "pool_id", pid, "func", "DeletePool")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
