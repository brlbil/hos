// SPDX-License-Identifier: MIT

// Package logger provides structured logging configuration for HOS services.
// It wraps httplog with customizable levels, output formats, and quiet down periods.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-chi/httplog/v2"
)

const (
	// LOG levels
	Debug = slog.LevelDebug
	Info  = slog.LevelInfo
	Warn  = slog.LevelWarn
	Error = slog.LevelError
	// disable all logging
	None = slog.Level(-99)
)

// NewLevel converts a string to slog.Level
func NewLevel(levelString string) (slog.Level, error) {
	switch strings.ToLower(levelString) {
	case "debug":
		return Debug, nil
	case "info":
		return Info, nil
	case "warn":
		return Warn, nil
	case "error":
		return Error, nil
	case "none":
		return None, nil
	default:
		return None, fmt.Errorf("unknown level %s", levelString)
	}
}

// opt holds logger configuration options
type opt struct {
	json            bool
	msgName         string
	levelName       string
	tags            map[string]string
	quietDownRoutes []string
	quietDownPeriod time.Duration
	level           slog.Level
	writer          io.Writer
}

// Option represents a logger configuration function
type Option func(o *opt) error

// WithLevel sets the logging level
func WithLevel(levelString string) Option {
	return func(options *opt) error {
		level, err := NewLevel(levelString)
		if err != nil {
			return err
		}
		options.level = level
		return nil
	}
}

// WithFieldNames customizes log field names
func WithFieldNames(message, level string) Option {
	return func(options *opt) error {
		options.msgName = message
		options.levelName = level
		return nil
	}
}

// WithQuiteDown configures quiet periods for specific routes
func WithQuiteDown(period time.Duration, paths ...string) Option {
	return func(options *opt) error {
		options.quietDownPeriod = period
		options.quietDownRoutes = paths
		return nil
	}
}

// WithTags adds custom tags to log entries
func WithTags(tags map[string]string) Option {
	return func(options *opt) error {
		options.tags = tags
		return nil
	}
}

// WithJSON enables JSON output format
func WithJSON(options *opt) error {
	options.json = true
	return nil
}

// New creates a new HTTP logger with specified options
func New(service string, opts ...Option) (*httplog.Logger, error) {
	options := &opt{
		msgName:   "message",
		levelName: "severity",
		level:     slog.LevelInfo,
		writer:    os.Stderr,
		quietDownRoutes: []string{
			"/",
			"/ping",
		},
		quietDownPeriod: time.Second * 10,
	}
	for _, optionFunc := range opts {
		if err := optionFunc(options); err != nil {
			return nil, err
		}
	}
	if options.level == None {
		options.writer = io.Discard
	}

	logger := httplog.NewLogger(service, httplog.Options{
		LogLevel: options.level,
		JSON:     options.json,
		Concise:  true,
		// RequestHeaders:   true,
		// ResponseHeaders:  true,
		MessageFieldName: options.msgName,
		LevelFieldName:   options.levelName,
		TimeFieldFormat:  time.RFC3339,
		Tags:             options.tags,
		QuietDownRoutes:  options.quietDownRoutes,
		QuietDownPeriod:  options.quietDownPeriod,
		// SourceFieldName: "source",
		Writer: options.writer,
	})

	return logger, nil
}
