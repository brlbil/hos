// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/crypto"
)

const (
	head  = "HEAD"
	get   = "GET"
	post  = "POST"
	put   = "PUT"
	del   = "DELETE"
	patch = "PATCH"
)

// httpError maps HOS error types to appropriate HTTP status codes and logs the error
func httpError(w http.ResponseWriter, log *slog.Logger, msg string, err error, args ...any) {
	logArgs := []any{}
	logArgs = append(logArgs, "error", err)
	logArgs = append(logArgs, args...)
	log.Error(msg, logArgs...)
	switch {
	case errors.Is(err, hos.ErrExist):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, hos.ErrNotEmpty):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, hos.ErrNotExist):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, hos.ErrNotEqual):
		http.Error(w, err.Error(), constant.HTTPStatusNotEqual)
	case errors.Is(err, hos.ErrNotAuthorized):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, hos.ErrInsufficientPermissions):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, hos.ErrNotAllowed):
		http.Error(w, err.Error(), constant.HTTPStatusNotAllowed)
	case errors.Is(err, hos.ErrBadRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, hos.ErrSizeRequired):
		http.Error(w, err.Error(), http.StatusLengthRequired)
	case errors.Is(err, hos.ErrContentTypeRequired):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	case errors.Is(err, hos.ErrContentTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, hos.ErrDecryption):
		http.Error(w, err.Error(), http.StatusExpectationFailed)
	case errors.Is(err, hos.ErrNotInitialized):
		http.Error(w, err.Error(), constant.HTTPStatusNotInitialized)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// getMutatedKey retrieves and mutates an encryption key for object encryption/decryption
func getMutatedKey(ctx context.Context, uid, key string, o *hos.Object, dbi *db.DB) ([]byte, error) {
	kid, encKey, err := enc.ID(uid, key)
	if err != nil {
		return nil, fmt.Errorf("generating encryption key id failed, %w", err)
	}
	k, err := db.Get[enc.Key](ctx, dbi, kid)
	if err != nil {
		return nil, fmt.Errorf("getting encryption key failed, %w", err)
	}
	k.Set(encKey)
	mutatedKey, err := k.Mutate(o.CreatedAt.UnixNano())
	if err != nil {
		return nil, fmt.Errorf("mutating key failed, %w", err)
	}
	return mutatedKey, nil
}

// fromRequest parses HTTP headers into typed structs with validation for pools and objects
func fromRequest[T any](r *http.Request) (*T, error) {
	t, err := header.Parse[T](r.Header)
	if err != nil {
		return nil, err
	}

	switch parsedType := any(t).(type) {
	case *hos.Pool:
		// only validate name when creating a pool
		if err := validate.Pool(parsedType.Name); err != nil && r.Method == put {
			return nil, errors.Join(err, hos.ErrBadRequest)
		}
		if parsedType.LinkedID == "" {
			// we do not care about none Linked pools
			break
		}
		// Let's remove unwanted bits from Linked pool
		parsedType.ReplicaCount = 0
		parsedType.Labels = nil
		parsedType.Permissions = nil

		return any(parsedType).(*T), nil
	case *hos.Object:
		// only validate name when creating a object
		if err := validate.Object(parsedType.Name); err != nil && r.Method == put {
			return nil, errors.Join(err, hos.ErrBadRequest)
		}
		if r.ContentLength > 0 && parsedType.Size < r.ContentLength {
			parsedType.Size = r.ContentLength
		}

		sizeNotReq := r.Header.Get(header.SizeUnknown) != ""
		if parsedType.Size == 0 && r.Method == put && !sizeNotReq {
			return nil, hos.ErrSizeRequired
		}

		if parsedType.ContentType == "" && r.Method == put {
			return nil, hos.ErrContentTypeRequired
		}

		if hash := r.Header.Get(header.OriginalHash); hash != "" {
			parsedType.Hash = hash
		}
	}

	return t, nil
}

// queryFuncs converts filter headers into database query functions
func queryFuncs(flt *filter.Headers) []db.QueryFunc {
	queryFunctions := []db.QueryFunc{}
	if flt.NamePrefix != "" {
		queryFunctions = append(queryFunctions, db.NamePrefix(flt.NamePrefix))
	}
	if len(flt.Range) == 2 {
		queryFunctions = append(queryFunctions, db.Range(flt.Range[0], flt.Range[1]))
	}
	if len(flt.Labels) > 0 {
		queryFunctions = append(queryFunctions, db.Labels(flt.Labels...))
	}

	return queryFunctions
}

// mergeKeys combines existing and new public keys, handling deletion with '!' prefix
func mergeKeys(oldKeys, newKeys []crypto.PublicKey) ([]crypto.PublicKey, error) {
	delList := []crypto.PublicKey{}

	// publicKey would not be a lot so looping over them is not a problem
	indexFn := func(key crypto.PublicKey) int {
		for i, ik := range oldKeys {
			if bytes.Equal(ik, key) {
				return i
			}
		}
		return -1
	}

	for _, key := range newKeys {
		if len(key) == 33 && key[0] != '!' {
			return nil, hos.ErrBadRequest
		}

		if key[0] == '!' {
			delList = append(delList, key[1:])
			continue
		}

		if index := indexFn(key); index == -1 {
			oldKeys = append(oldKeys, key)
		}
	}

	for _, dk := range delList {
		index := indexFn(dk)
		if index == -1 {
			return nil, fmt.Errorf("key not found %w", hos.ErrBadRequest)
		}
		oldKeys = append(oldKeys[:index], oldKeys[index+1:]...)
	}

	return oldKeys, nil
}

// mergeMaps combines maps (labels/permissions), handling deletion with '!' key prefix
func mergeMaps[M ~map[string]V, V string | hos.Permission](old, new M) (M, error) {
	if new == nil {
		return old, nil
	}
	if old == nil {
		old = map[string]V{}
	}

	delList := []string{}

	for key, val := range new {
		if after, ok := strings.CutPrefix(key, "!"); ok {
			delList = append(delList, after)
			continue
		}
		old[key] = val
	}

	for _, dk := range delList {
		if _, ok := old[dk]; !ok {
			return nil, fmt.Errorf("%s is not exist %w", dk, hos.ErrBadRequest)
		}
		delete(old, dk)
	}

	return old, nil
}

// writeHeaders serializes struct fields to HTTP response headers
func writeHeaders(w http.ResponseWriter, a any) {
	for k, v := range header.Serialize(a) {
		w.Header().Set(k, v)
	}
}

// writeJSON encodes and writes JSON response with proper headers
func writeJSON(w http.ResponseWriter, v any) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(buf.Len()), 10))

	w.Write(buf.Bytes()) //nolint:errcheck
}

// calHash calculates SHA256 hash of concatenated strings
func calHash(ss ...string) string {
	hash := sha256.New()
	for _, s := range ss {
		hash.Write([]byte(s))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
