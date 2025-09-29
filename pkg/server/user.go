// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/id"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

// setupUsers initializes default system users during server startup
func (s *Server) setupUsers() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// check admin user, if not exist Create it
	for _, u := range []hos.User{{Name: constant.AdminUser, ID: id.Admin}} {
		if err := db.Create(ctx, s.db, &u); err != nil && !errors.Is(err, hos.ErrExist) {
			return fmt.Errorf("creating user %s failed: %w", u.Name, err)
		}

		if err := s.fs.CreateUser(ctx, &u); err != nil && !errors.Is(err, hos.ErrExist) {
			return fmt.Errorf("creating user %s failed: %w", u.Name, err)
		}
	}

	return nil
}

// createUser handles PUT /api/v1/user - registers a new user (admin only)
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "CreateUser")
	s.log.Debug("decoding user from request", "func", "CreateUser")
	var user hos.User
	if err := render.DecodeJSON(r.Body, &user); err != nil {
		httpError(w, s.log.Logger, "decoding user failed",
			fmt.Errorf("parsing json failed %s %w", err.Error(), hos.ErrBadRequest), "func", "CreateUser")
		return
	}

	// if ID is calculated wrong fix it
	user.ID = id.Gen(user.Name)
	switch user.ID {
	case id.Admin, id.Anonymous:
		// this function cannot be called for admin or anon user
		httpError(w, s.log.Logger, "checking users failed", fmt.Errorf("%w on %s", hos.ErrNotAllowed, user.ID), "func", "CreateUser")
		return
	}

	if err := validate.User(user.Name); err != nil {
		httpError(w, s.log.Logger, "user name validation failed",
			errors.Join(err, hos.ErrBadRequest), "func", "CreateUser")
	}

	// check if we have public key, if it more than one, or one key has valid length
	s.log.Debug("checking public key count and length", "func", "CreateUser")
	if l := len(user.PublicKeys); l == 0 || l > 1 || len(user.PublicKeys[0]) != 32 {
		httpError(w, s.log.Logger, "create user", fmt.Errorf("public key is invalid %w", hos.ErrBadRequest))
		return
	}

	if err := db.Create(r.Context(), s.db, &user); err != nil {
		httpError(w, s.log.Logger, "creating user failed", err, "func", "CreateUser")
		return
	}

	if err := s.fs.CreateUser(r.Context(), &user); err != nil {
		httpError(w, s.log.Logger, "creating user failed", err, "func", "CreateUser")
		return
	}

	w.WriteHeader(201)
}

func (s *Server) updateUserKeys(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "UpdateUserKeys")
	s.log.Debug("decoding user from request", "func", "UpdateUserKeys")
	var u hos.User
	if err := render.DecodeJSON(r.Body, &u); err != nil {
		httpError(w, s.log.Logger, "decoding user failed",
			fmt.Errorf("parsing json failed %s %w", err.Error(), hos.ErrBadRequest), "func", "UpdateUserKeys")
		return
	}

	ctx := r.Context()
	user := getUser(ctx)
	// if admin user is not registered its public key
	// then deny any attempt to update any user other than admin
	s.log.Debug("checking admin user set up", "func", "UpdateUserKeys")
	if len(user.PublicKeys) == 0 && u.ID != id.Admin {
		httpError(w, s.log.Logger, "checking admin user set up failed",
			fmt.Errorf("admin user has no key, %w", hos.ErrNotAllowed), "func", "UpdateUserKeys")
		return
	}

	// confirm user name and ID match
	s.log.Debug("conforming user name and id", "func", "UpdateUserKeys")
	if id := id.Gen(u.Name); id != u.ID {
		err := fmt.Errorf("user (name %s, id %s) does not match ID %s %w", u.Name, u.ID, id, hos.ErrBadRequest)
		httpError(w, s.log.Logger, "conforming user name and id failed", err, "func", "UpdateUserKeys")
		return
	}

	if u.ID == id.Anonymous {
		httpError(w, s.log.Logger, "checking for anonymous user", hos.ErrNotAllowed, "func", "UpdateUserKeys")
		return
	}

	ub4, err := db.Get[hos.User](ctx, s.db, u.ID)
	if err != nil {
		httpError(w, s.log.Logger, "getting user keys failed", err, "user_id", u.ID, "func", "UpdateUserKeys")
		return
	}

	s.log.Debug("merging user keys", "user_id", u.ID, "func", "UpdateUserKeys")
	newKeys, err := mergeKeys(ub4.PublicKeys, u.PublicKeys)
	if err != nil {
		httpError(w, s.log.Logger, "merging user keys failed", err, "user_id", u.ID, "func", "UpdateUserKeys")
		return
	}
	u.PublicKeys = newKeys

	if err := db.Update(ctx, s.db, &u); err != nil {
		httpError(w, s.log.Logger, "updating user keys failed", err, "user_id", u.ID, "func", "UpdateUserKeys")
		return
	}

	if err := s.fs.UpdateUser(ctx, &u); err != nil {
		httpError(w, s.log.Logger, "updating user keys failed", err, "user_id", u.ID, "func", "UpdateUserKeys")
		return
	}

	w.WriteHeader(204)
}

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "GetUsers")
	ctx := r.Context()
	users, err := db.List[hos.User](ctx, s.db, "")
	if err != nil {
		httpError(w, s.log.Logger, "listing users failed", err, "func", "GetUsers")
		return
	}

	writeJSON(w, users)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	s.log.Debug("starting", "func", "DeleteUser")
	uid := chi.URLParam(r, "uid")
	switch uid {
	case id.Admin, id.Anonymous:
		// not allowed to delete these two accounts
		httpError(w, s.log.Logger, "deleting builtin users failed", hos.ErrNotAllowed, "func", "DeleteUser")
		return
	}

	ctx := r.Context()
	if err := db.Delete[hos.User, hos.Pool](ctx, s.db, uid); err != nil {
		httpError(w, s.log.Logger, "deleting user failed", err, "user_id", uid, "func", "DeleteUser")
		return
	}

	if err := s.fs.DeleteUser(ctx, uid); err != nil {
		httpError(w, s.log.Logger, "deleting user failed", err, "user_id", uid, "func", "DeleteUser")
		return
	}

	w.WriteHeader(204)
}
