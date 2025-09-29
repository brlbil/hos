// SPDX-License-Identifier: MIT

package enc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
	"github.com/brlbil/hos/pkg/id"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

const ErrReadFailure = hos.ConstError("read failure")

// ID generates a key ID from user ID and encryption key
func ID[T []byte | string](uid string, key T) (string, []byte, error) {
	var (
		encKey []byte
		err    error
	)

	switch v := any(key).(type) {
	case []byte:
		encKey = v
	case string:
		encKey, err = validate.EncryptionKey[string, []byte](v)
		if err != nil {
			return "", nil, err
		}
	}

	return id.GenSHA([]byte(uid), encKey), encKey, nil
}

// Key represents an encryption key with metadata and encrypted data
type Key struct {
	CreatedAt time.Time `json:"created_at" diff:"ignore"`
	ID        string    `json:"id"  boltholdKey:"ID"`
	UserID    string    `json:"user_id" boltholdIndex:"UserID" parentID:"true"`
	Data      []byte    `json:"data,omitempty"`

	encKey []byte
}

// Create derives a new key from this key using the provided encryption key
func (k *Key) Create(newkey string) (*Key, error) {
	encKey, err := validate.EncryptionKey[string, []byte](newkey)
	if err != nil {
		return nil, err
	}

	masterKey, err := k.unsealKey()
	if err != nil {
		return nil, err
	}

	return create(k.UserID, encKey, masterKey)
}

// Set assigns the encryption key for unsealing operations
func (k *Key) Set(encKey []byte) {
	k.encKey = encKey
}

// unsealKey decrypts and extracts the master key from encrypted data
func (k *Key) unsealKey() ([]byte, error) {
	randomData, err := decrypt(k.encKey, k.Data)
	if err != nil {
		return nil, err
	}
	skip := binary.BigEndian.Uint64(randomData[:8])
	masterKey := make([]byte, 32)
	copy(masterKey[:], randomData[skip:skip+32])
	randomData = nil

	return masterKey, nil
}

// Mutate derives a mutation-specific key
func (k *Key) Mutate(mutation int64) ([]byte, error) {
	masterKey, err := k.unsealKey()
	if err != nil {
		return nil, err
	}
	mutationBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(mutationBytes, uint64(mutation))
	return pbkdf2.Key(masterKey, mutationBytes, 4096, 32, sha256.New), nil
}

// Create generates a new encryption key for a user
func Create(uid, key string) (*Key, error) {
	encKey, err := validate.EncryptionKey[string, []byte](key)
	if err != nil {
		return nil, err
	}

	keyPair := make([]byte, 64)
	n, err := rand.Read(keyPair)
	if err != nil {
		return nil, errors.Join(ErrReadFailure, err)
	}
	if n != 64 {
		return nil, fmt.Errorf("%w red key length %d, expected 64", ErrReadFailure, n)
	}

	masterKey, err := scrypt.Key(keyPair[:32], keyPair[32:], 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}

	return create(uid, encKey, masterKey)
}

// create internal function for key creation with random data embedding
func create(uid string, encKey, masterKey []byte) (*Key, error) {
	// create some random data
	randomData := make([]byte, 256)
	_, err := rand.Read(randomData)
	if err != nil {
		return nil, err
	}

	// lets set a random start point
	randomBigInt, err := rand.Int(rand.Reader, big.NewInt(224))
	if err != nil {
		return nil, errors.Join(ErrReadFailure, err)
	}
	skipCount := max(uint64(randomBigInt.Int64()), 8)
	binary.BigEndian.PutUint64(randomData, skipCount)

	// copy master key to position
	copy(randomData[skipCount:skipCount+32], masterKey[:])

	// enctypt the rd
	encryptedData, err := encrypt(encKey, randomData)
	if err != nil {
		return nil, err
	}

	return &Key{CreatedAt: time.Now(), ID: id.GenSHA([]byte(uid), encKey), UserID: uid, Data: encryptedData, encKey: encKey}, nil
}
