// SPDX-License-Identifier: MIT

// Package crypto provides Ed25519 cryptographic operations for HOS authentication.
// It handles key generation, parsing, signing, and verification for user authentication tokens.
package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"

	"filippo.io/edwards25519"
	"github.com/brlbil/hos/pkg/id"
)

// GenerateKey creates a new Ed25519 key pair
func GenerateKey() (PublicKey, PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, err
	}

	return PublicKey(publicKey[:]), PrivateKey(privateKey[:]), nil
}

// PrivateKey represents an Ed25519 private key
type PrivateKey []byte

// ParsePrivateKey parses a base64-encoded private key string
func ParsePrivateKey(s string) (PrivateKey, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("private key parsing error: %w", err)
	}

	if len(decodedBytes) != 32 {
		return nil, fmt.Errorf("private key expected length is 32, got %d", len(decodedBytes))
	}

	publicKey := genPublicFromPrivate(decodedBytes)
	privateKey := make([]byte, 64)
	copy(privateKey, decodedBytes)
	copy(privateKey[32:], publicKey)

	return PrivateKey(privateKey[:]), nil
}

// String returns the base64-encoded private key (32 bytes only)
func (p PrivateKey) String() string {
	if p == nil {
		return string(p)
	}
	return base64.StdEncoding.EncodeToString(p[:32])
}

// PublicKey extracts the public key from this private key
func (p PrivateKey) PublicKey() (PublicKey, error) {
	if keyLength := len(p); keyLength != 64 {
		return nil, fmt.Errorf("private key expected length is 32, got %d", keyLength)
	}

	publicKeyBytes := make([]byte, 32)
	copy(publicKeyBytes, p[32:])

	return PublicKey(publicKeyBytes), nil
}

// SignUser creates an authentication token by signing user ID and hash
func (p PrivateKey) SignUser(user string) string {
	userID := make([]byte, 40)
	copy(userID, []byte(id.Gen(user)))
	userHash := sha256.Sum256([]byte(user))
	copy(userID[8:], userHash[:])

	privateKey := ed25519.PrivateKey(p[:])
	signature := ed25519.Sign(privateKey, userID[:])

	message := make([]byte, 104)
	copy(message, userID[:])
	copy(message[40:], signature)

	return base64.StdEncoding.EncodeToString(message)
}

// PublicKey represents an Ed25519 public key
type PublicKey []byte

// ParsePublicKey parses a base64-encoded public key string
func ParsePublicKey(s string) (PublicKey, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("public key parsing error: %w", err)
	}

	if len(decodedBytes) != 32 {
		return nil, fmt.Errorf("public key expected length is 32, got %d", len(decodedBytes))
	}

	return PublicKey(decodedBytes[:]), nil
}

// String returns the base64-encoded public key
func (p PublicKey) String() string {
	if p == nil {
		return string(p)
	}
	return base64.StdEncoding.EncodeToString(p[:])
}

// MarshalYAML implements YAML marshaling for configuration files
func (p PublicKey) MarshalYAML() (any, error) {
	return p.String(), nil
}

// VerifyUser verifies an authentication token signature against user ID
func (p PublicKey) VerifyUser(uid, sig []byte) bool {
	// PublicKey might be empty
	if len(p) != 32 {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(p[:]), uid[:], sig[:])
}

// Split extracts user ID and signature from a base64-encoded authentication token
func Split(s string) ([]byte, []byte, error) {
	decodedMessage, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, nil, err
	}

	userID := make([]byte, 40)
	signature := make([]byte, 64)

	copy(userID, decodedMessage[:40])
	copy(signature, decodedMessage[40:])

	return userID, signature, nil
}

// genPublicFromPrivate derives the public key from private key bytes
func genPublicFromPrivate(privateKeyBytes []byte) []byte {
	hash := sha512.Sum512(privateKeyBytes)
	scalar, err := edwards25519.NewScalar().SetBytesWithClamping(hash[:32])
	if err != nil {
		panic("ed25519: internal error: setting scalar failed")
	}
	point := (&edwards25519.Point{}).ScalarBaseMult(scalar)

	return point.Bytes()
}
