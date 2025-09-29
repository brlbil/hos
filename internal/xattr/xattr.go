// SPDX-License-Identifier: MIT

// Package xattr provides extended attributes storage for HOS metadata.
// It handles JSON serialization with gzip compression for filesystem extended attributes.
package xattr

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"syscall"

	"github.com/brlbil/hos"
	"github.com/pkg/xattr"
)

// encode compresses and encodes data for extended attribute storage
func encode[T any](data *T) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	gzipWriter, err := gzip.NewWriterLevel(buffer, gzip.BestCompression)
	if err != nil {
		return nil, err
	}

	encoder := json.NewEncoder(gzipWriter)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	gzipWriter.Close()

	encodedData := make([]byte, base64.StdEncoding.EncodedLen(len(buffer.Bytes())))
	base64.StdEncoding.Encode(encodedData, buffer.Bytes())
	return encodedData, nil
}

// decode decompresses and decodes data from extended attribute storage
func decode[T any](data []byte) (*T, error) {
	decodedData := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	if _, err := base64.StdEncoding.Decode(decodedData, data); err != nil {
		return nil, err
	}

	gzipReader, err := gzip.NewReader(bytes.NewBuffer(decodedData))
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(gzipReader)

	var result T
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Set encodes and stores data in extended attributes
func Set[T any](path string, data *T) error {
	encodedData, err := encode(data)
	if err != nil {
		return err
	}

	if err := xattr.Set(path, metaDataKey, encodedData); err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return fmt.Errorf("%s %w", path, hos.ErrNotExist)
		}
		return err
	}

	return nil
}

// Get retrieves and decodes data from extended attributes
func Get[T any](path string) (*T, error) {
	xattrData, err := xattr.Get(path, metaDataKey)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return nil, fmt.Errorf("%s %w", path, hos.ErrNotExist)
		}
		return nil, err
	}

	return decode[T](xattrData)
}

// List returns all extended attribute names for a path
func List(path string) ([]string, error) {
	return xattr.List(path)
}
