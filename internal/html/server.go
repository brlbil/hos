// SPDX-License-Identifier: MIT

// Package html provides an HTML web interface for HOS.
// It serves a web UI for managing pools, objects, and users through a browser.
package html

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/logger"
	"github.com/brlbil/hos/pkg/client"
	"github.com/brlbil/hos/pkg/id"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v2"
)

// Server represents an HTML web interface server for HOS
type Server struct {
	srv     *http.Server
	userID  string
	clt     *client.Client
	log     *httplog.Logger
	reqOpts []client.Options
}

// New creates a new HTML server with client connection and optional user impersonation
func New(addr, logLevel string, clt *client.Client, impUser string) (*Server, error) {
	logger, err := logger.New("hos-html", logger.WithLevel(logLevel), logger.WithQuiteDown(0))
	if err != nil {
		return nil, err
	}

	// certain this does not return error so we can ignore it
	_ = clt.Reconfigure(client.PinContentServer)
	server := &Server{
		srv: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		clt:    clt,
		userID: id.Gen(clt.User()),
		log:    logger,
	}
	if impUser != "" {
		server.userID = id.Gen(impUser)
		server.reqOpts = []client.Options{client.OnBehalf(impUser)}
	}

	return server, nil
}

func (s *Server) Start() error {
	router := chi.NewRouter()
	router.Use(httplog.RequestLogger(s.log, nil))
	router.Use(middleware.Recoverer)
	router.Get("/favicon.ico", s.favicon)
	router.Get("/", s.listPools)
	router.Get("/{pool_name}/*", s.handleObjects)

	s.srv.Handler = router

	return s.srv.ListenAndServe()
}

func (s *Server) Stop() error {
	return s.srv.Shutdown(context.Background())
}

func (s *Server) listPools(w http.ResponseWriter, r *http.Request) {
	pools, err := s.clt.ListPools(
		r.Context(),
		append(s.reqOpts, client.IgnoreErrors(hos.ErrNotExist))...,
	)
	if err != nil {
		s.log.Error("list pools", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := page("/", getItems(pools)).Render(r.Context(), w); err != nil {
		s.log.Error("render page", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleObjects(w http.ResponseWriter, r *http.Request) {
	poolName := chi.URLParam(r, "pool_name")

	prefix, err := url.QueryUnescape(chi.URLParam(r, "*"))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	s.log.Debug("handling objects", "pool name", poolName, "object path", prefix)

	if prefix == "" || strings.HasSuffix(prefix, "/") {
		s.listObjects(w, r, poolName, prefix)
		return
	}
	s.serveObject(w, r, poolName, prefix)
}

func (s *Server) listObjects(w http.ResponseWriter, r *http.Request, poolName string, prefix string) {
	poolID := id.Gen(s.userID, poolName)
	s.log.Debug("listing objects", "pool name", poolName, "ignore errors", hos.ErrNotExist, "filter prefix", prefix)
	objects, err := s.clt.ListObjects(r.Context(), poolID,
		append(
			s.reqOpts,
			client.IgnoreErrors(hos.ErrNotExist),
			filter.NamePrefix(prefix),
			client.ObjectDirectoryListing(prefix),
		)...,
	)
	if err != nil {
		s.log.Error("list objects", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := page(fmt.Sprintf("%s/%s", poolName, prefix), getItems(objects)).Render(r.Context(), w); err != nil {
		s.log.Error("render page", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) serveObject(w http.ResponseWriter, r *http.Request, poolName string, objectName string) {
	poolID := id.Gen(s.userID, poolName)
	objectID := id.Gen(poolID, objectName)

	headers := map[string]string{}
	for headerKey := range r.Header {
		headers[headerKey] = r.Header.Get(headerKey)
	}

	s.log.Debug("getting object", "pool name", poolName, "object name", objectName)
	object, err := s.clt.GetContent(r.Context(), poolID, objectID,
		append(s.reqOpts, client.IgnoreErrors(hos.ErrNotExist), client.Headers(headers))...)
	if err != nil {
		s.log.Error("getting object", "error", err, "pool name", poolName, "object name", objectName)
		if errors.Is(err, hos.ErrNotExist) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer object.Close()

	w.Header().Set("Content-Type", object.ContentType)
	if _, err := io.Copy(w, object); err != nil {
		s.log.Error("coping content", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not found", http.StatusNotFound)
}
