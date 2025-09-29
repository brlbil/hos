// SPDX-License-Identifier: MIT

// Package service provides service lifecycle management with graceful shutdown.
// It handles service startup and shutdown with signal handling support.
package service

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Service represents a service with start/stop lifecycle
type Service interface {
	Start() error
	Stop() error
}

// Run starts a service and handles graceful shutdown on SIGINT/SIGTERM
func Run(s Service) error {
	errCh := make(chan error)
	go func() {
		errCh <- s.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("service exited with %w", err)
		}
		return nil
	case <-sigCh:
		return s.Stop()
	}
}
