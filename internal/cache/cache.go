// SPDX-License-Identifier: MIT

// Package cache provides generic in-memory caching with TTL support.
// It uses sync.Map for concurrent access with expiration-based cleanup.
package cache

import (
	"sync"
	"time"
)

// node represents a cached value with expiration timestamp
type node[V any] struct {
	value   V
	expires int64
}

// Get retrieves a cached value by key, checking expiration
func Get[K, V any](cacheMap *sync.Map, key K) (V, bool) {
	var emptyValue V
	genericValue, exists := cacheMap.Load(key)
	if !exists {
		return emptyValue, false
	}

	value, ok := genericValue.(node[V])
	if !ok {
		return emptyValue, false
	}

	if value.expires > 0 && time.Now().UnixNano() > value.expires {
		Delete(cacheMap, key)
		return emptyValue, false
	}

	return value.value, true
}

// Size returns the number of cached entries
func Size(cacheMap *sync.Map) int {
	var count int
	cacheMap.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Set stores a value with optional TTL expiration
func Set[K, V any](cacheMap *sync.Map, key K, value V, duration time.Duration) {
	var expirationTime int64

	if duration > 0 {
		expirationTime = time.Now().Add(duration).UnixNano()
	}

	cacheMap.Store(key, node[V]{
		value:   value,
		expires: expirationTime,
	})
}

// Delete removes a cached entry by key
func Delete[K any](cacheMap *sync.Map, key K) {
	cacheMap.Delete(key)
}
