// SPDX-License-Identifier: MIT

package cache

import (
	"sync"
	"testing"
	"time"
)

func TestCache_Get(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  int
		d    time.Duration
	}{
		{
			name: "never",
			key:  "one",
			val:  1,
		},
		{
			name: "timeout",
			key:  "two",
			val:  2,
			d:    time.Millisecond,
		},
	}

	m := &sync.Map{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Set(m, tt.key, tt.val, tt.d)
			got, found := Get[string, int](m, tt.key)
			if !found {
				t.Errorf("key %s not found", tt.key)
			}
			if got != tt.val {
				t.Errorf("key %s value = %v, expected %d", tt.key, got, tt.val)
			}
			time.Sleep(time.Millisecond * 3)
			_, found = Get[string, int](m, tt.key)
			if found && tt.d != time.Duration(0) {
				t.Errorf("key %s found, timeout failed", tt.key)
			}
			if !found && tt.d == time.Duration(0) {
				t.Errorf("key %s not found, expected to be exists", tt.key)
			}
			if s := Size(m); s != 1 {
				t.Errorf("size %d, expected 1", s)
			}
		})
	}
}
