// SPDX-License-Identifier: MIT

package tests

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/enc"
	"github.com/brlbil/hos/pkg/id"
)

const (
	Key1     = "g1GIfVjWfAJywrNW/G93ahq35XGwUebI+ljIMCBhS1E="
	Key2     = "6RKUr2XYDVaWwcb8ZNtrCPfBT32Jfre6qco9FpOQPTE="
	Key3     = "2xmatmXudlUUmTxMI5cD5lCmWsoxIhtPIAJHi44FupE="
	ShortKey = "ht7obVTl3YyCr94ekWlUr+ymeSlwAO3aBLpE1oVmOw==" // this one's length is 31
)

func Key(key string) []byte {
	b, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		panic(fmt.Errorf("base64 decode failure %w", err))
	}
	return b
}

func UserKey(user, key string) hos.Key {
	uid := id.Gen(user)
	ukey := Key(key)
	return hos.Key{Signature: id.GenSHA([]byte(uid), ukey), UserID: uid}
}

func UserKeyData(user, key string) enc.Key {
	uid := id.Gen(user)
	ukey := Key(key)
	return enc.Key{ID: id.GenSHA([]byte(uid), ukey), UserID: uid, Data: bytes.Repeat([]byte{0}, 296)}
}
