// SPDX-License-Identifier: MIT

package id

import (
	"testing"
)

func TestGen(t *testing.T) {
	tests := []struct {
		name string
		want string
		s    []string
	}{
		{name: "admin", s: []string{"admin"}, want: Admin},
		{name: "anon", s: []string{""}, want: Anonymous},
		{name: "pool1", s: []string{Admin, "pool1"}, want: "db7202fb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Gen(tt.s...); got != tt.want {
				t.Errorf("Gen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenSHA(t *testing.T) {
	tests := []struct {
		name string
		want string
		b    [][]byte
	}{
		{name: "admin", b: [][]byte{[]byte("admin")}, want: "8c6976e5b5410415bde908bd4dee15dfb167a9c873fc4bb8a81f6f2ab448a918"},
		{name: "pool1", b: [][]byte{[]byte(Admin), []byte("pool1")}, want: "65c6c057b9f4804e25ac3f985601795f2e4f0f7ab87ee135fe1efd63ff4c1680"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenSHA(tt.b...); got != tt.want {
				t.Errorf("Gen() = %v, want %v", got, tt.want)
			}
		})
	}
}
