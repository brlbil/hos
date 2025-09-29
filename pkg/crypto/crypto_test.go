// SPDX-License-Identifier: MIT

package crypto

import (
	"bytes"
	"testing"
)

func TestSignVerify(t *testing.T) {
	pub, prv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	prvs := prv.String()
	if len(prvs) != 44 {
		t.Fatalf("PrivateKey.String() want len = 44, got %d", len(prvs))
	}

	pubs := pub.String()
	if len(pubs) != 44 {
		t.Fatalf("PublicKey.String() want len = 44, got %d", len(pubs))
	}

	prv2, err := ParsePrivateKey(prvs)
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}

	pub2, err := ParsePublicKey(pubs)
	if err != nil {
		t.Fatalf("ParsePublicKey() error = %v", err)
	}

	if !bytes.Equal(prv, prv2) {
		t.Fatalf(" Private keys is not equal")
	}

	if !bytes.Equal(pub, pub2) {
		t.Fatalf(" Private keys is not equal")
	}

	msg := prv2.SignUser("jhsdgfjhsgdfjshd")

	uid, sig, err := Split(msg)
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	if !pub2.VerifyUser(uid, sig) {
		t.Error("Verify() failed")
	}
}
