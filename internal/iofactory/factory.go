// SPDX-License-Identifier: MIT

// Package iofactory provides reusable IO reader factories.
// It creates multiple ReadCloser instances from the same source.
package iofactory

import (
	"io"
	"os"
)

// ReadClosers creates multiple ReadCloser instances from the same source
type ReadClosers interface {
	// New creates a ReadCloser for the specified server
	New(string) (io.ReadCloser, error)
}

// FileReadClosers creates a ReadClosers factory for file paths.
// Returns error if file doesn't exist.
func FileReadClosers(filePath string) (ReadClosers, error) {
	if _, err := os.Stat(filePath); err != nil {
		return nil, err
	}
	return &fileReadClosers{filePath: filePath}, nil
}

// MustFileReadClosers creates a ReadClosers factory.
// Panics if file doesn't exist.
func MustFileReadClosers(filePath string) ReadClosers {
	if _, err := os.Stat(filePath); err != nil {
		panic(err)
	}
	return &fileReadClosers{filePath: filePath}
}

type fileReadClosers struct {
	filePath string
}

// New implements ReadClosers interface by opening the file
func (f *fileReadClosers) New(_ string) (io.ReadCloser, error) {
	file, err := os.Open(f.filePath)
	if err != nil {
		return nil, err
	}
	return file, nil
}
