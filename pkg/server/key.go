// SPDX-License-Identifier: MIT

package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/header"
	"github.com/go-chi/chi/v5"
)

// createKey handles PUT /api/v1/key - creates new encryption keys or derives keys from existing ones
func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "CreateKey")
	s.log.Debug("reading encryption key from request", "func", "CreateKey")

	ctx := r.Context()
	user := getUser(ctx)

	var newKey *enc.Key

	encKeyHeader := r.Header.Get(header.EncryptionKey)
	newEncKeyHader := r.Header.Get(header.EncryptionNewKey)
	// enc key header is not empty so create a new from an existing one
	if encKeyHeader != "" {
		kid, encKey, err := enc.ID(user.ID, encKeyHeader)
		if err != nil {
			httpError(w, s.log.Logger, "getting key id failed", err, "func", "CreateKey")
			return
		}

		existingKey, err := db.Get[enc.Key](ctx, s.db, kid)
		if err != nil {
			httpError(w, s.log.Logger, "getting key from db failed", err, "func", "CreateKey", "key_id", kid)
			return
		}
		existingKey.Set(encKey)

		newKey, err = existingKey.Create(newEncKeyHader)
		if err != nil {
			httpError(w, s.log.Logger, "creating new key failed", err, "func", "CreateKey")
			return
		}
	} else {
		keyCount, err := db.Count[enc.Key](ctx, s.db, user.ID)
		if err != nil {
			httpError(w, s.log.Logger, "getting key count failed", err, "func", "CreateKey")
			return
		}

		if keyCount > 0 {
			err = fmt.Errorf("keys %w", hos.ErrExist)
			httpError(w, s.log.Logger, "creating new key failed", err, "func", "CreateKey", "key_count", keyCount)
			return
		}

		newKey, err = enc.Create(user.ID, newEncKeyHader)
		if err != nil {
			httpError(w, s.log.Logger, "creating new key failed", err, "func", "CreateKey")
			return
		}
	}

	if err := db.Create(ctx, s.db, newKey); err != nil {
		httpError(w, s.log.Logger, "creating key failed", err, "func", "CreateKey")
		return
	}

	if err := s.fs.CreateKey(ctx, newKey); err != nil {
		httpError(w, s.log.Logger, "creating key failed", err, "func", "CreateKey")
		return
	}

	w.WriteHeader(201)
}

// restoreKey handles PUT /api/v1/key/data - restores an encryption key from backup data
func (s *Server) restoreKey(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "RestoreKey")
	defer r.Body.Close()

	ctx := r.Context()
	user := getUser(ctx)

	var key enc.Key
	err := json.NewDecoder(r.Body).Decode(&key)
	if err != nil {
		httpError(w, s.log.Logger, "reading key failed", err, "func", "RestoreKey")
		return
	}
	// validate key
	if l := len(key.Data); l != 296 {
		httpError(w, s.log.Logger, "key is too short", hos.ErrBadRequest, "func", "RestoreKey")
		return
	}
	if key.UserID != user.ID {
		msg := fmt.Sprintf("user %s does not match key user %s", user.ID, key.UserID)
		httpError(w, s.log.Logger, msg, hos.ErrBadRequest, "func", "RestoreKey")
		return
	}

	if err := db.Create(ctx, s.db, &key); err != nil {
		httpError(w, s.log.Logger, "creating key failed", err, "func", "RestoreKey")
		return
	}

	if err := s.fs.CreateKey(ctx, &key); err != nil {
		httpError(w, s.log.Logger, "creating key failed", err, "func", "RestoreKey")
		return
	}

	w.WriteHeader(201)
}

// getKeys handles GET /api/v1/key - returns user's key metadata (without sensitive data)
func (s *Server) getKeys(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "GetKeys")
	ctx := r.Context()
	user := getUser(ctx)
	ekeys, err := db.List[enc.Key](ctx, s.db, user.ID, db.SortByFields("CreatedAt"))
	if err != nil {
		httpError(w, s.log.Logger, "listing keys failed", err, "func", "GetKeys")
		return
	}

	keys := make([]hos.Key, len(ekeys))
	for i, k := range ekeys {
		keys[i] = hos.Key{Signature: k.ID, UserID: k.UserID, CreatedAt: k.CreatedAt}
	}

	writeJSON(w, keys)
}

// getKeyData handles GET /api/v1/key/data - returns complete key data for backup purposes
func (s *Server) getKeyData(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "GeyKeyData")
	ctx := r.Context()
	user := getUser(ctx)

	keys, err := db.List[enc.Key](ctx, s.db, user.ID, db.SortByFields("CreatedAt"))
	if err != nil {
		httpError(w, s.log.Logger, "getting keys failed", err, "func", "GetKeyData")
		return
	}

	writeJSON(w, keys)
}

// deleteKey handles DELETE /api/v1/key/{kid} - deletes an encryption key (prevents deleting last key)
func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "DeleteKey")
	kid := chi.URLParam(r, "kid")
	ctx := r.Context()
	user := getUser(ctx)

	keys, err := db.List[enc.Key](ctx, s.db, user.ID)
	if err != nil {
		httpError(w, s.log.Logger, "listing keys failed", err, "func", "DeleteKeys")
		return
	}
	if len(keys) == 1 && keys[0].ID == kid {
		httpError(w, s.log.Logger, "last key cannot be deleted", hos.ErrNotAllowed, "func", "DeleteKey")
		return
	}

	if err := db.Delete[enc.Key, enc.Key](ctx, s.db, kid); err != nil {
		httpError(w, s.log.Logger, "deleting key failed", err, "key_id", kid, "func", "DeleteKey")
		return
	}

	if err := s.fs.DeleteKey(ctx, kid); err != nil {
		httpError(w, s.log.Logger, "deleting key failed", err, "key_id", kid, "func", "DeleteKey")
		return
	}

	w.WriteHeader(204)
}
