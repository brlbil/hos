// SPDX-License-Identifier: MIT

package xattr

import (
	"errors"
	"testing"

	"github.com/brlbil/hos"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/xattr"
)

type testStruct struct {
	C map[string]string
	B string
	A int64
}

func TestXattr(t *testing.T) {
	if _, err := List("."); err != nil {
		t.Skip("not supporting xattr")
	}

	ts := &testStruct{A: 13, B: "B", C: map[string]string{"X": "Y"}}

	if err := Set("xattr_test.go", ts); err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if err := Set("xxxxx", ts); !errors.Is(err, hos.ErrNotExist) {
		t.Errorf("expected error not exist, got %s", err)
	}

	ts2, err := Get[testStruct]("xattr_test.go")
	if err != nil {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if _, err := Get[testStruct]("xxxxx"); !errors.Is(err, hos.ErrNotExist) {
		t.Errorf("expected error <nil>, got %s", err)
	}

	if diff := cmp.Diff(ts, ts2); diff != "" {
		t.Error(diff)
	}
	// clean up

	if err := xattr.Remove("xattr_test.go", metaDataKey); err != nil {
		t.Error(err)
	}
}
