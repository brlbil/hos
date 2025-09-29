// SPDX-License-Identifier: MIT

// Package enc provides ChaCha20-Poly1305 encryption utilities for HOS.
// It handles symmetric encryption and decryption operations.
package enc

import (
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

// encrypt encrypts a message using ChaCha20-Poly1305 with random nonce
func encrypt(key, msg []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	// Select a random nonce, and leave capacity for the ciphertext.
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(msg)+aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	return aead.Seal(nonce, nonce, msg, nil), nil
}

// decrypt decrypts a ChaCha20-Poly1305 encrypted message
func decrypt(key, encMsg []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	if len(encMsg) < aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	// Split nonce and ciphertext.
	nonce, ciphertext := encMsg[:aead.NonceSize()], encMsg[aead.NonceSize():]

	// Decrypt the message and check it wasn't tampered with.
	return aead.Open(nil, nonce, ciphertext, nil)
}
