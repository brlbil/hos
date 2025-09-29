// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/pkg/crypto"
	"github.com/brlbil/hos/pkg/id"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v2"
)

const (
	prefix = "Bearer "

	read  hos.Permission = "r"
	write hos.Permission = "w"
)

type contextKey struct {
	name string
}

var (
	userKey   = &contextKey{"user"}
	poolKey   = &contextKey{"pool"}
	destKey   = &contextKey{"dest_pool"}
	objectKey = &contextKey{"object"}
	byPassKey = &contextKey{"bypass"}
)

// getLogger extracts the structured logger from the request context
func getLogger(ctx context.Context) *slog.Logger {
	log := httplog.LogEntry(ctx)
	return log
}

// getUser retrieves the authenticated user from the request context
func getUser(ctx context.Context) *hos.User {
	log := getLogger(ctx)
	log.Debug("getting user from context")
	u, _ := ctx.Value(userKey).(hos.User)
	return &u
}

// getPool retrieves the current pool from the request context
func getPool(ctx context.Context) *hos.Pool {
	log := getLogger(ctx)
	log.Debug("getting pool from context")
	p, _ := ctx.Value(poolKey).(hos.Pool)
	return &p
}

// getDestPool retrieves the destination pool from the request context (used for move operations)
func getDestPool(ctx context.Context) *hos.Pool {
	log := getLogger(ctx)
	log.Debug("getting destination pool from context")
	v := ctx.Value(destKey)
	if v == nil {
		return nil
	}
	p, _ := v.(hos.Pool)
	return &p
}

// getObject retrieves the current object from the request context
func getObject(ctx context.Context) *hos.Object {
	log := getLogger(ctx)
	log.Debug("getting object from context")
	o, _ := ctx.Value(objectKey).(hos.Object)
	return &o
}

// isByPass checks if authentication should be bypassed for this request
func isByPass(ctx context.Context) bool {
	log := getLogger(ctx)
	log.Debug("getting by_pass from context")
	return ctx.Value(byPassKey) != nil
}

// redirect middleware handles pool linking by redirecting requests to linked pools.
func (s *Server) redirect(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		s.log.Debug("staring redirect")
		ctx := r.Context()
		pid := chi.URLParam(r, "pid")

		pool, err := db.Get[hos.Pool](ctx, s.db, pid)
		if err != nil {
			httpError(w, s.log.Logger, "getting pool from db", fmt.Errorf("pool %w", err))
			return
		}

		s.log.Debug("setting pool to context")
		ctx = context.WithValue(ctx, poolKey, *pool)

		var dpool *hos.Pool
		if id := chi.URLParam(r, "did"); id != "" {
			dpool, err = db.Get[hos.Pool](ctx, s.db, id)
			if err != nil {
				httpError(w, s.log.Logger, "getting destination pool from db", fmt.Errorf("destination pool %w", err))
				return
			}

			s.log.Debug("setting destination pool to context")
			ctx = context.WithValue(ctx, destKey, *dpool)
		}

		oid := chi.URLParam(r, "oid")

		method := r.Method
		// do not redirect DELETE and POST opt on Pools
		if oid == "" && (method == del || method == post) {
			s.log.Debug("delete on pool, not redirecting")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if pool.LinkedID != "" && r.Header.Get(header.NoRedirect) == "" {
			// only owner of the link can use it
			u := getUser(ctx)
			if pool.UserID != u.ID {
				httpError(w, s.log.Logger, "checking permission for a linked pool", fmt.Errorf("%w, owner only", hos.ErrNotAllowed))
				return
			}

			if dpool != nil && dpool.LinkedID != "" && dpool.UserID != u.ID {
				httpError(w, s.log.Logger, "checking permission for a linked pool", fmt.Errorf("%w, owner only", hos.ErrNotAllowed))
				return
			}

			redirectURL := *r.URL
			redirectURL.Path = strings.ReplaceAll(r.URL.Path, pool.ID, pool.LinkedID)
			if dpool != nil && dpool.LinkedID != "" {
				redirectURL.Path = strings.ReplaceAll(redirectURL.Path, dpool.ID, dpool.LinkedID)
			}
			s.log.Debug("redirecting", "url", redirectURL.String())
			http.Redirect(w, r, redirectURL.String(), http.StatusPermanentRedirect)
			return
		}

		s.log.Debug("no redirection")
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(handlerFunc)
}

// authenticate middleware validates Bearer tokens and sets user context.
func (s *Server) authenticate(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		s.log.Debug("starting authentication")
		ctx := r.Context()
		if isByPass(ctx) {
			s.log.Debug("authentication is by passed")
			next.ServeHTTP(w, r)
			return
		}

		userHash, signature, tokSts := parseTokenAuth(r)
		switch tokSts {
		case tokenTooShort:
			httpError(w, s.log.Logger, "parsing auth token failed, token is short", fmt.Errorf("no %s token %w", prefix, hos.ErrNotAuthorized))
			return
		case tokenMalformed:
			httpError(w, s.log.Logger, "parsing auth token failed, token is malformed", fmt.Errorf("malformed token %w", hos.ErrNotAuthorized))
			return
		case tokenNotExist:
			// anonymous access
			s.log.Debug("authentication is not set anonymous access")
			ctx := context.WithValue(ctx, userKey, hos.User{ID: id.Anonymous})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		user, err := db.Get[hos.User](ctx, s.db, string(userHash[:8]))
		// user is not exist
		if err != nil {
			httpError(w, s.log.Logger, "token is not exist", hos.ErrNotAuthorized)
			return
		}

		if user.ID == id.Admin && len(user.PublicKeys) == 0 && r.URL.Path == constant.UserAPIPrefix {
			// if user is admin and the admin user does not have Public key
			// then allow update user key call
			if r.Method == post {
				ctx := context.WithValue(ctx, userKey, *user)
				next.ServeHTTP(w, r.WithContext(ctx))
			}

			httpError(w, s.log.Logger, "cluster", hos.ErrNotInitialized)
			return
		}

		// verify user signature
		s.log.Debug("verifying user", "user_id", user.ID, "user_name", user.Name)
		if err := verifyUser(user, userHash, signature); err != nil {
			httpError(w, s.log.Logger, "verifying user failed", errors.Join(err, hos.ErrNotAuthorized))
			return
		}

		if user.Name == "admin" {
			// if request has impersonate header change the user
			if un := r.Header.Get(header.OnBehalf); un != "" {
				s.log.Debug("admin impersonates another user", "user_id", id.Gen(un), "user_name", un)
				user, err = db.Get[hos.User](ctx, s.db, id.Gen(un))
				if err != nil {
					httpError(w, s.log.Logger, "getting user failed", hos.ErrNotAuthorized, "user_name", un)
					return
				}
				user.OnBehalf()
			}
		}

		s.log.Debug("setting user to context", "user_id", user.ID, "user_name", user.Name)
		ctx = context.WithValue(ctx, userKey, *user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
	return http.HandlerFunc(handlerFunc)
}

// verifyUser validates the user's cryptographic signature against their public keys
func verifyUser(u *hos.User, userHash, signature []byte) error {
	if len(u.PublicKeys) == 0 {
		return fmt.Errorf("user %s does not have any publicKeys", u.Name)
	}

	for _, pk := range u.PublicKeys {
		if pk.VerifyUser(userHash, signature) {
			return nil
		}
	}

	return fmt.Errorf("user %s auth token cannot be verified", u.Name)
}

// pool: write permission gives other users the ability to create objects and delete objects, but not modify the Pool
// pool: read permission gives users the ability to get metadata of the pool and list the objects
// pool owner: can still do every operation on all the objects created by other users
// anonymous user: can only perform read operations

// authorize middleware enforces permission-based access control for pools and objects.
// Checks read/write permissions based on HTTP method and user ownership.
func (s *Server) authorize(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		s.log.Debug("starting authorization")
		ctx := r.Context()
		method := r.Method
		user := getUser(ctx)

		op := read
		did := chi.URLParam(r, "did")
		if method == del || method == put || method == post || (method == patch && did != "") {
			op = write
		}

		oid := chi.URLParam(r, "oid")
		uid := user.ID

		pool := getPool(ctx)

		if pool.LinkedID != "" && method == post {
			httpError(w, s.log.Logger, "change operation is not allowed on linked pools",
				fmt.Errorf("change %w on linked pools", hos.ErrNotAllowed), "pool_id", pool.ID)
			return
		}

		// move object check destination pool permissions
		if did != "" {
			dpool := getDestPool(ctx)
			// pool owner has full access
			if dpool.UserID != uid && !checkPermission(user.Name, op, dpool.Permissions, s.log.Logger) {
				httpError(w, s.log.Logger, "check permissions failed", hos.ErrInsufficientPermissions,
					"user_id", uid, "pool_id", dpool.ID, "pool_name", dpool.Name)
				return
			}
		}

		// requested url is pool
		if oid == "" {
			// only owner can delete and edit the pools
			if pool.UserID != uid && (method == del || method == post) {
				met := del
				if method != del {
					met = "edit"
				}
				httpError(w, s.log.Logger, "user does not own the pool",
					hos.ErrInsufficientPermissions, "http_method", met, "user_id",
					pool.UserID, "pool_id", pool.ID, "pool_name", pool.Name)
				return
			}
			// pool owner has full access
			if pool.UserID == uid || checkPermission(user.Name, op, pool.Permissions, s.log.Logger) {
				s.log.Debug("authorization successful", "user_id", uid, "pool_id", pool.ID, "pool_name", pool.Name)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			httpError(w, s.log.Logger, "check permissions failed", hos.ErrInsufficientPermissions,
				"user_id", uid, "pool_id", pool.ID, "pool_name", pool.Name)
			return
		}

		obj, err := db.Get[hos.Object](ctx, s.db, oid)
		if err != nil {
			httpError(w, s.log.Logger, "getting object failed", err)
			return
		}

		s.log.Debug("setting object to context", "object_id", obj.ID, "object_name", obj.Name)
		ctx = context.WithValue(ctx, objectKey, *obj)
		// object owner or pool owner has full access
		if obj.UserID == uid || pool.UserID == uid || checkPermission(user.Name, op, pool.Permissions, s.log.Logger) {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		httpError(w, s.log.Logger, "check permissions failed", hos.ErrInsufficientPermissions, "user_id", uid, "pool_id", pool.ID,
			"pool_name", pool.Name, "object_id", obj.ID, "object_name", obj.Name)
	}

	return http.HandlerFunc(handlerFunc)
}

// by pass auth for certain paths
// authNotRequired middleware bypasses authentication for specific endpoints like /healthz
func authNotRequired(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == get && r.URL.Path == "/healthz" {
			ctx := context.WithValue(r.Context(), byPassKey, true)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(handlerFunc)
}

// adminOnly middleware restricts access to admin user only
func adminOnly(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user := getUser(ctx)
		log := getLogger(ctx)
		if user.Name != "admin" {
			httpError(w, log, "check admin user", fmt.Errorf("%w, only admin", hos.ErrNotAuthorized))
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(handlerFunc)
}

// blockAdmin middleware prevents admin user from accessing certain endpoints
func blockAdmin(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user := getUser(ctx)
		log := getLogger(ctx)
		if user.Name == "admin" {
			httpError(w, log, "block admin user", fmt.Errorf("admin is %w", hos.ErrNotAuthorized))
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(handlerFunc)
}

// blockAnonymous middleware prevents unauthenticated access
func blockAnonymous(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user := getUser(ctx)
		log := getLogger(ctx)
		if user.Name == "" {
			httpError(w, log, "block anonymous access", fmt.Errorf("anonymous access is %w", hos.ErrNotAuthorized))
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(handlerFunc)
}

type localOnly struct {
	ips []string
}

// newLocalOnly creates a middleware that restricts access to local network interfaces only
func newLocalOnly(addr string) *localOnly {
	local, _, _ := strings.Cut(addr, ":")
	if local != "0.0.0.0" {
		return &localOnly{ips: []string{local}}
	}
	addrs := []string{}
	ifcs, err := net.Interfaces()
	if err != nil {
		return &localOnly{ips: []string{}}
	}
	for _, ifc := range ifcs {
		netAddrs, _ := ifc.Addrs()
		for _, addr := range netAddrs {
			s, _, _ := strings.Cut(addr.String(), "/")
			addrs = append(addrs, s)
		}
	}

	return &localOnly{ips: addrs}
}

// handler enforces local-only access by checking client IP addresses
func (l *localOnly) handler(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := strings.Split(r.RemoteAddr, ":")[0]

		ctx := r.Context()
		log := getLogger(ctx)
		log.Debug("checking remote addrs", "remote_addr", remoteAddr, "server_ips", l.ips)

		if !slices.Contains(l.ips, remoteAddr) {
			httpError(w, log, "only local access is allowed", fmt.Errorf("remote access is %w", hos.ErrNotAuthorized))
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(handlerFunc)
}

// checkPermission validates if a user has the required permission level.
// Admin users have full access, anonymous users can only read.
func checkPermission(user string, op hos.Permission, per map[string]hos.Permission, log *slog.Logger) bool {
	log.Debug("checking permission for user " + user)
	if user == constant.AdminUser {
		return true
	}

	// anonymous user can only perform get operations
	if user == "" && op == write {
		return false
	}

	if p, ok := per[constant.Everyone]; ok && p >= op {
		return true
	}
	if p, ok := per[user]; ok && p >= op {
		return true
	}

	return false
}

type tokenStatus int8

const (
	tokenOK tokenStatus = iota
	tokenNotExist
	tokenTooShort
	tokenMalformed
)

// parseTokenAuth extracts and validates Bearer token from Authorization header
func parseTokenAuth(r *http.Request) ([]byte, []byte, tokenStatus) {
	log := getLogger(r.Context())
	log.Debug("parsing auth token")
	auth := r.Header.Get("Authorization")
	if auth == "" {
		log.Debug("auth token is not exists in header")
		return nil, nil, tokenNotExist
	}

	if len(auth) < len(prefix) {
		log.Debug("auth token does not have prefix TOKEN: " + auth)
		return nil, nil, tokenTooShort
	}

	userHash, signature, err := crypto.Split(auth[len(prefix):])
	if err != nil {
		log.Debug("parsing token failed", "error", err)
		return nil, nil, tokenMalformed
	}

	return userHash, signature, tokenOK
}

// parseToken extracts user hash from token string
func parseToken(token string) (string, tokenStatus) {
	userHash, _, err := crypto.Split(token)
	if err != nil {
		return "", tokenMalformed
	}

	return string(userHash), tokenOK
}
