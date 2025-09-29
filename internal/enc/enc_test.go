// SPDX-License-Identifier: MIT

package enc

import (
	"crypto/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func newKeyB(t *testing.T) []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEnc(t *testing.T) {
	key := newKeyB(t)
	msg := `This is a test text
that is a multiline
and there is nothing special about it.
`

	encMsg, err := encrypt(key, []byte(msg))
	if err != nil {
		t.Error("encrypt", err)
	}
	if string(encMsg) == msg {
		t.Fatal("text is not encrypted")
	}

	msg2, err := decrypt(key, encMsg)
	if err != nil {
		t.Error("decrypt", err)
	}
	if diff := cmp.Diff(msg, string(msg2)); diff != "" {
		t.Error(diff)
	}
}
