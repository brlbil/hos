// SPDX-License-Identifier: MIT

// Package server provides the HOS (Home Object Storage) server implementation.
// It handles HTTP requests for managing pools, objects, and users with authentication
// and authorization. Each server manages local storage in a single directory tree.
package server

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/cert"
	"github.com/brlbil/hos/internal/db"
	"github.com/brlbil/hos/internal/fs"
	"github.com/brlbil/hos/internal/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v2"
)

// Server represents the HOS HTTP server with integrated storage, database, and authentication.
type Server struct {
	conf     *Config
	srv      *http.Server
	fs       *fs.FS
	db       *db.DB
	optCount *atomic.Int64
	caCert   []byte
	log      *httplog.Logger
}

func (s *Server) String() string {
	return s.srv.Addr
}

// New creates a new HOS server instance with the provided configuration.
// It initializes the filesystem, database, TLS certificates, and routes.
// Call Start() to begin serving requests.
func New(conf *Config) (*Server, error) {
	if err := conf.Check(); err != nil {
		return nil, err
	}

	log, err := logger.New("hosd",
		logger.WithLevel(conf.LogLevel),
		logger.WithQuiteDown(time.Second*10, "/", "/healthz"),
	)
	if err != nil {
		return nil, err
	}

	rootDir, err := filepath.Abs(conf.RootDir)
	if err != nil {
		return nil, err
	}

	f, err := fs.New(rootDir, log.Logger)
	if err != nil {
		return nil, fmt.Errorf("initializing fs failed: %w", err)
	}

	db, err := db.New(conf.RootDir, log.Logger)
	if err != nil {
		return nil, err
	}

	log.Debug("creating CA certificate")
	ca, pem, err := cert.CreateCA(conf.RootDir)
	if err != nil {
		return nil, err
	}

	log.Debug("creating server certificate")
	tlsConf, err := cert.CreateServerCert(ca)
	if err != nil {
		return nil, err
	}

	s := &Server{
		srv: &http.Server{
			Addr:              conf.Address,
			TLSConfig:         tlsConf,
			ReadHeaderTimeout: 10 * time.Second,
		},
		conf:     conf,
		caCert:   pem,
		fs:       f,
		db:       db,
		optCount: &atomic.Int64{},
		log:      log,
	}

	log.Debug("creating admin user")
	if err := s.setupUsers(); err != nil {
		return nil, err
	}

	log.Debug("setting routes")
	s.setupRoutes()

	return s, nil
}

func (s *Server) optCounter(next http.Handler) http.Handler {
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		log := httplog.LogEntry(r.Context())
		log.Debug("increasing operation counter")
		s.optCount.Add(1)
		defer func() {
			log.Debug("decreasing operation counter")
			s.optCount.Add(-1)
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(handlerFunc)
}

func (s *Server) setupRoutes() {
	r := chi.NewRouter()
	r.Use(middleware.StripSlashes)
	r.Use(httplog.RequestLogger(s.log))
	r.Use(authNotRequired)
	r.Use(s.authenticate)
	r.Use(s.optCounter)
	r.Use(middleware.Recoverer)

	checkAuthR := r.With(blockAdmin, s.redirect, s.authorize)
	blockAnonR := r.With(blockAdmin, blockAnonymous)
	adminOnlyR := r.With(adminOnly)

	r.Head("/api/v1", s.getServerInfo)

	// List pools
	blockAnonR.Get("/api/v1", s.listPools)
	// List objects
	checkAuthR.Get("/api/v1/{pid}", s.listObjects)

	// Get pool
	checkAuthR.Head("/api/v1/{pid}", s.getPool)
	// Get object information
	checkAuthR.Head("/api/v1/{pid}/{oid}", s.getObject)

	// Get object content
	checkAuthR.Get("/api/v1/{pid}/{oid}", s.getObjectContent)

	// Create a pool
	blockAnonR.Put("/api/v1", s.createPool)
	// Create an object
	checkAuthR.Put("/api/v1/{pid}", s.createObject)

	// Edit a pool
	checkAuthR.Post("/api/v1/{pid}", s.editPool)
	// Edit an object
	checkAuthR.Post("/api/v1/{pid}/{oid}", s.editObject)

	// Move an object
	checkAuthR.Patch("/api/v1/{pid}/{oid}/{did}", s.moveObject)

	// Copy an object
	checkAuthR.Patch("/api/v1/{pid}/{oid}", s.copyObject)

	// Delete a pool, pool must be empty
	checkAuthR.Delete("/api/v1/{pid}", s.deletePool)
	// Delete an object
	checkAuthR.Delete("/api/v1/{pid}/{oid}", s.deleteObject)

	// Fuzzy Find Pools and Objects
	blockAnonR.Get("/api/v1/find", s.find)

	adminOnlyR.Put("/api/v1/user", s.createUser)
	adminOnlyR.Post("/api/v1/user", s.updateUserKeys)
	adminOnlyR.Get("/api/v1/user", s.getUsers)
	adminOnlyR.Delete("/api/v1/user/{uid}", s.deleteUser)

	blockAnonR.Put("/api/v1/key", s.createKey)
	blockAnonR.Get("/api/v1/key", s.getKeys)
	blockAnonR.Delete("/api/v1/key/{kid}", s.deleteKey)
	blockAnonR.Get("/api/v1/key/data", s.getKeyData)
	blockAnonR.Put("/api/v1/key/data", s.restoreKey)

	r.Get("/ca", s.serveCA)
	r.Get("/healthz", s.health)

	allowLocalOnly := r.With(blockAnonymous, newLocalOnly(s.conf.Address).handler)
	allowLocalOnly.Get("/api/v1/config", s.config)

	s.srv.Handler = r
}

// Start starts the HTTPS server and begins listening for requests
func (s *Server) Start() error {
	s.log.Info("starting server", "address", s.srv.Addr)
	err := s.srv.ListenAndServeTLS("", "")
	if err != nil {
		s.log.Error("server start", "error", err)
	}

	return err
}

// Stop gracefully shuts down the server and closes database connections
func (s *Server) Stop() error {
	s.log.Info("stopping server")

	if err := s.srv.Shutdown(context.Background()); err != nil {
		return err
	}
	return s.db.Close()
}

// serveCA serves the server's CA certificate for client verification
func (s *Server) serveCA(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(s.caCert)
}

// health provides a health check endpoint returning server status
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	raddr := strings.Split(r.RemoteAddr, ":")[0]
	fmt.Fprintf(w, "Status:OK RemoteAddr:%s\n", raddr)
}

// config returns the server configuration (localhost access only)
func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.conf)
}

// getServerInfo returns disk usage and operation count
func (s *Server) getServerInfo(w http.ResponseWriter, r *http.Request) {
	statfs, err := s.fs.GetDiskInfo()
	if err != nil {
		httpError(w, s.log.Logger, "getting server info failed", err)
		return
	}

	writeHeaders(w, &hos.ServerInfo{Statfs: *statfs, Operations: s.optCount.Load()})
	w.WriteHeader(http.StatusNoContent)
}
