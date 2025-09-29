// SPDX-License-Identifier: MIT

// Package dir provides directory walking and path information utilities.
// It handles recursive directory traversal with content type detection.
package dir

import (
	"path/filepath"
)

// MAYBE change function signature to have option functions

// Walk traverses directories and returns path information with optional recursion
func Walk(contentType string, recursive bool, paths ...string) ([]PathInfo, error) {
	if contentType != "" {
		getContentType = func(filePath string) (string, error) { return contentType, nil }
	} else {
		getContentType = readContentType
	}

	pathMapper := pathMap{fileMap: map[string]PathInfo{}, dirMap: map[string]struct{}{}}
	// if not recursive just read the files given
	if !recursive {
		if err := pathMapper.read(paths...); err != nil {
			return nil, err
		}
	} else {
		for _, path := range paths {
			if err := filepath.WalkDir(path, pathMapper.walkFunc); err != nil {
				return nil, err
			}
		}
	}

	return pathMapper.pathInfo(), nil
}
