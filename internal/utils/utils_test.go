// SPDX-License-Identifier: MIT

package utils

import (
	"bytes"
	"io"
	"testing"
	"time"
)

type tb []byte

type ts string

type sta struct {
	AD time.Time `diff:"ignore"`
	AE map[string]ts
	AA string
	AF []tb
	AC int64
	AB int
}

type stb struct {
	BA time.Time
}

func TestDiff(t *testing.T) {
	stime := time.Date(2023, 5, 14, 11, 13, 34, 3453, time.UTC)

	tests := []struct {
		name    string
		a       any
		b       any
		wantErr string
	}{
		{
			name: "nil",
		},
		{
			name:    "not same type",
			a:       &sta{},
			b:       &stb{},
			wantErr: "type *utils.sta, is not same type as *utils.stb",
		},
		{
			name: "ignored field",
			a: &sta{
				AD: time.Now(),
			},
			b: &sta{
				AD: time.Now().Add(time.Hour),
			},
		},
		{
			name: "different",
			a: &sta{
				AA: "first",
				AB: 1,
				AC: 1,
				AD: time.Now(),
				AE: map[string]ts{
					"first": "1",
					"third": "2",
				},
				AF: []tb{[]byte("first"), []byte("second"), []byte("third")},
			},
			b: &sta{
				AA: "second",
				AB: 2,
				AC: 2,
				AD: time.Now(),
				AE: map[string]ts{
					"second": "2",
					"first":  "-1",
				},
				AF: []tb{[]byte("first"), []byte("third")},
			},
			wantErr: `sta not equal
  AE:
    = first: 1 != -1
    + third: 2
    - second: 2
  AA: first != second
  AF:
    = 1: 7365636f6e64 != 7468697264
    + 2: 7468697264
  AC: 1 != 2
  AB: 1 != 2`,
		},
		{
			name: "different slice",
			a: &sta{
				AF: []tb{[]byte("third"), []byte("first")},
			},
			b: &sta{
				AF: []tb{[]byte("first"), []byte("second"), []byte("third")},
			},
			wantErr: `sta not equal
  AF:
    = 1: 7468697264 != 7365636f6e64
    - 2: 7468697264`,
		},
		{
			name: "diff length slice A",
			a:    &sta{},
			b: &sta{
				AF: []tb{[]byte("first")},
			},
			wantErr: `sta not equal
  AF:
    - 0: 6669727374`,
		},
		{
			name: "diff length slice B",
			a: &sta{
				AF: []tb{[]byte("first")},
			},
			b: &sta{},
			wantErr: `sta not equal
  AF:
    + 0: 6669727374`,
		},
		{
			name: "same",
			a: &sta{
				AA: "first",
				AB: 1,
				AC: 1,
				AD: time.Now(),
				AE: map[string]ts{
					"first":  "1",
					"second": "2",
				},
				AF: []tb{[]byte("first"), []byte("second")},
			},
			b: &sta{
				AA: "first",
				AB: 1,
				AC: 1,
				AD: time.Now(),
				AE: map[string]ts{
					"second": "2",
					"first":  "1",
				},
				AF: []tb{[]byte("first"), []byte("second")},
			},
		},
		{
			name: "same sb",
			a: &stb{
				BA: stime,
			},
			b: &stb{
				BA: stime,
			},
		},
		{
			name: "diff sb",
			a: &stb{
				BA: stime,
			},
			b: &stb{
				BA: stime.Add(time.Hour),
			},
			wantErr: `stb not equal
  BA: 2023-05-14T11:13:34.000003453Z != 2023-05-14T12:13:34.000003453Z`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errVal := ""
			err := Diff(tt.a, tt.b)
			if err != nil {
				errVal = err.Error()
			}
			if errVal != tt.wantErr {
				t.Errorf("Diff() error = '%s', wantErr '%s'", errVal, tt.wantErr)
			}
		})
	}
}

func TestCountWriter(t *testing.T) {
	br := bytes.NewReader(bytes.Repeat([]byte{55, 56, 57, 58, 59}, 1000))
	cw := CountWriter(0)
	_, _ = io.Copy(&cw, br)
	if cw != 5000 {
		t.Errorf("expected count is 5000, got %d", cw)
	}
}
