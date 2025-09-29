// SPDX-License-Identifier: MIT

package server

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"

	"github.com/brlbil/hos/internal/logger"
)

// Config represents the server configuration.
type Config struct {
	RootDir  string `json:"root_dir"`  // Root directory for data storage
	Address  string `json:"address"`   // Server bind address and port
	LogLevel string `json:"log_level"` // Logging level (debug, info, warn, error)
}

// Check validates the configuration
func (c *Config) Check() error {
	if c == nil {
		return errors.New("server configuration is not defined")
	}

	rd, err := filepath.Abs(c.RootDir)
	if err != nil {
		return fmt.Errorf("config root directory error: %w", err)
	}
	c.RootDir = rd

	ip, err := net.ResolveTCPAddr("tcp", c.Address)
	if err != nil {
		return fmt.Errorf("config address error: %w", err)
	}
	c.Address = ip.String()

	_, err = logger.NewLevel(c.LogLevel)
	if err != nil {
		return fmt.Errorf("config %w", err)
	}

	return nil
}
