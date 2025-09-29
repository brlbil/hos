//go:build !linux

package xattr

// metaDataKey is the extended attribute key for HOS metadata (non-Linux)
const metaDataKey = "hos.metadata"
