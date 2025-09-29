// SPDX-License-Identifier: MIT

// Package id provides ID generation utilities for HOS system entities.
// It generates consistent, deterministic IDs for users, pools, and objects.
package id

import (
	"crypto/sha256"
	"fmt"
	"hash/crc32"
)

// Predefined system user IDs
const (
	Admin     = "880e0d76" // Admin user ID
	Anonymous = "00000000" // Anonymous user ID
)

// Gen generates a deterministic 8-character hex ID from input strings using CRC32
func Gen(s ...string) string {
	hash := crc32.NewIEEE()
	for _, i := range s {
		hash.Write([]byte(i))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// GenSHA generates a SHA256 hash from input byte slices
func GenSHA(bb ...[]byte) string {
	hash := sha256.New()
	for _, b := range bb {
		hash.Write(b)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
