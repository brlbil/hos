// SPDX-License-Identifier: MIT

package server

import (
	"fmt"
	"net/http"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/db"
)

// find handles GET /api/v1/find - performs fuzzy search across pools and objects by name
func (s *Server) find(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "Find")
	ctx := r.Context()
	// p := getPool(ctx)

	s.log.Debug("parsing query parameters from url query", "func", "Find")
	name := r.URL.Query().Get("name")
	if name == "" {
		err := fmt.Errorf("no name parameter, %w", hos.ErrBadRequest)
		httpError(w, s.log.Logger, "getting search parameters failed", err, "func", "Find")
		return
	}

	// find pools
	pools, err := db.Find[hos.Pool](ctx, s.db, name)
	if err != nil {
		httpError(w, s.log.Logger, "finding pools failed", err, "func", "Find")
		return
	}

	objs, err := db.Find[hos.Object](ctx, s.db, name)
	if err != nil {
		httpError(w, s.log.Logger, "finding objects failed", err, "func", "Find")
		return
	}

	ret := []hos.Object{}
	for _, p := range pools {
		ret = append(ret, hos.Object{ID: p.ID, Name: p.Name})
	}
	for _, o := range objs {
		ret = append(ret, hos.Object{ID: o.ID, Name: o.Name, PoolID: o.PoolID})
	}

	writeJSON(w, ret)
}
