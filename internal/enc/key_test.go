// SPDX-License-Identifier: MIT

package enc

import (
	"errors"
	"testing"
	"time"

	"github.com/brlbil/hos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var encKeys = []string{
	"lV9ww8hz1lOPmHUPYIRWp9mFTZf6phfEhRqvuliSVJ0=",
	"diwGg/krPw9nG5JuFLOT/zeJY/+3zjBmuioZV9DJxTw=",
	"Pr+hbNNIGU0QWqf1EQefakHsMkuYZmiooj4+i0ykFOg=",
	"jpCWq/2IR37vvlRAooLc2tCe06ZNyvDT+AR25fzfQDOf",
}

const uid = "b9851374"

func TestKeyCreate(t *testing.T) {
	tests := []struct {
		want    *Key
		wantErr error
		name    string
		encKey  string
		useB4   bool
	}{
		{
			name:    "wrong key",
			encKey:  encKeys[3],
			wantErr: hos.ErrBadRequest,
		},
		{
			name:   "first",
			encKey: encKeys[0],
			want: &Key{
				ID:     "800ee2ea8afa61086a890c64fc46f9c3e41b429e10ee180afe4187eac59e69be",
				UserID: uid,
			},
		},
		{
			name:   "second",
			encKey: encKeys[1],
			want: &Key{
				ID:     "bd73c84c3340bcde021e84ca99cdbd2a0acffdf9141d7617754699b609323d08",
				UserID: uid,
			},
			useB4: true,
		},
		{
			name:   "third",
			encKey: encKeys[2],
			want: &Key{
				ID:     "6ffa5af0d6ba5351db3104a23ccc72c21ea96ad9eb54bb3fb11de17676e0bd37",
				UserID: uid,
			},
			useB4: true,
		},
	}

	var b4key *Key

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				result *Key
				err    error
			)

			if !tt.useB4 {
				result, err = Create(uid, tt.encKey)
			} else {
				result, err = b4key.Create(tt.encKey)
			}
			defer func() {
				if err == nil {
					b4key = result
				}
			}()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("%s != %s", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(result, tt.want, cmpopts.IgnoreUnexported(Key{}), cmpopts.IgnoreFields(Key{}, "Data", "CreatedAt")); diff != "" {
				t.Error(diff)
			}

			if b4key == nil {
				return
			}

			if cmp.Equal(b4key.Data, result.Data) {
				t.Error("data fileds should not be equal")
			}

			b4k, be := b4key.unsealKey()
			if be != nil {
				t.Error(be)
				return
			}
			k, e := result.unsealKey()
			if e != nil {
				t.Error(e)
				return
			}
			if diff := cmp.Diff(b4k, k); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestMutate(t *testing.T) {
	// create a key
	t1 := time.Now()
	key1, err := Create(uid, encKeys[2])
	if err != nil {
		t.Fatal(err)
	}
	key2, err := key1.Create(encKeys[0])
	if err != nil {
		t.Fatal(err)
	}
	t2 := time.Now()

	k1, err := key1.Mutate(t1.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	k2, err := key1.Mutate(t2.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	k3, err := key1.Mutate(t1.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	k4, err := key2.Mutate(t2.UnixNano())
	if err != nil {
		t.Fatal(err)
	}

	if cmp.Equal(k1, k2) {
		t.Error("k1, k2 equal")
	}
	if diff := cmp.Diff(k1, k3); diff != "" {
		t.Error(diff)
	}
	if diff := cmp.Diff(k2, k4); diff != "" {
		t.Error(diff)
	}
}

func BenchmarkMutate(b *testing.B) {
	key, err := Create(uid, encKeys[2])
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now().UnixNano()

	for b.Loop() {
		_, _ = key.Mutate(now)
	}
}
