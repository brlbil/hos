// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_parseArg(t *testing.T) {
	tests := []struct {
		fl          *argFlags
		want        *argRes
		name        string
		uid         string
		arg         string
		wantOptsLen int
		wantErr     bool
	}{
		{
			name: "pool id",
			arg:  "122bc828",
			fl:   &argFlags{pool: true, id: true},
			want: &argRes{poolID: "122bc828"},
		},
		{
			name: "pool or obj id",
			arg:  "122bc828",
			fl:   &argFlags{poolObj: true, id: true},
			want: &argRes{poolID: "122bc828"},
		},
		{
			name: "pool name",
			uid:  "d87f7e0c",
			arg:  "test",
			fl:   &argFlags{pool: true},
			want: &argRes{poolID: "122bc828"},
		},
		{
			name: "pool or object name",
			uid:  "d87f7e0c",
			arg:  "test",
			fl:   &argFlags{poolObj: true},
			want: &argRes{poolID: "122bc828"},
		},
		{
			name:    "pool id fail",
			arg:     "122bc82k",
			fl:      &argFlags{pool: true, id: true},
			wantErr: true,
		},
		{
			name:    "pool name fail",
			uid:     "d87f7e0c",
			arg:     "1test",
			fl:      &argFlags{pool: true},
			wantErr: true,
		},
		{
			name:    "user pool id",
			arg:     "test@122bc828",
			fl:      &argFlags{pool: true, id: true, userAt: true},
			wantErr: true,
		},
		{
			name: "user pool name",
			arg:  "test@test",
			fl:   &argFlags{pool: true, userAt: true},
			want: &argRes{poolID: "122bc828", poolName: "test"},
		},
		{
			name:        "label pool id",
			uid:         "d87f7e0c",
			arg:         "122bc828",
			fl:          &argFlags{id: true, pool: true, labels: []string{"color!=green"}},
			want:        &argRes{poolID: "122bc828"},
			wantOptsLen: 1,
		},
		{
			name:        "recursive",
			uid:         "d87f7e0c",
			arg:         "122bc828",
			fl:          &argFlags{id: true, pool: true, recursive: true},
			want:        &argRes{poolID: "122bc828"},
			wantOptsLen: 1,
		},
		{
			name: "id pool or obj",
			arg:  "122bc828/3d83c42a",
			fl:   &argFlags{id: true, poolObj: true},
			want: &argRes{poolID: "122bc828", objID: "3d83c42a"},
		},
		{
			name:        "label pool or object id",
			uid:         "d87f7e0c",
			arg:         "122bc828",
			fl:          &argFlags{id: true, poolObj: true, labels: []string{"color!=green"}},
			want:        &argRes{poolID: "122bc828"},
			wantOptsLen: 1,
		},
		{
			name: "label pool/object id",
			uid:  "d87f7e0c",
			arg:  "122bc828/3d83c42a",
			fl:   &argFlags{id: true, poolObj: true, recursive: true, labels: []string{"color!=green"}},
			want: &argRes{poolID: "122bc828", objID: "3d83c42a"},
		},
		{
			name: "id pool/obj",
			arg:  "122bc828/3d83c42a",
			fl:   &argFlags{id: true, poolObj: true},
			want: &argRes{poolID: "122bc828", objID: "3d83c42a"},
		},
		{
			name:    "id pool/obj fail",
			arg:     "122bc828/3j83c42a",
			fl:      &argFlags{id: true, poolObj: true},
			wantErr: true,
		},
		{
			name:    "id pool/obj glob fail",
			arg:     "122bc828/201e13ce/ads/sdf///...",
			fl:      &argFlags{id: true},
			wantErr: true,
		},
		{
			name:    "label parse fail",
			arg:     "test",
			fl:      &argFlags{labels: []string{"not=valid"}},
			wantErr: true,
		},
		{
			name: "pool/obj",
			uid:  "d87f7e0c",
			arg:  "test/test1",
			fl:   &argFlags{},
			want: &argRes{poolID: "122bc828", objID: "201e13ce", objPath: "test1"},
		},
		{
			name: "pool or obj",
			uid:  "d87f7e0c",
			arg:  "test/test1",
			fl:   &argFlags{poolObj: true},
			want: &argRes{poolID: "122bc828", objID: "201e13ce", objPath: "test1"},
		},
		{
			name: "pool or obj with label",
			uid:  "d87f7e0c",
			arg:  "test/test1",
			fl:   &argFlags{poolObj: true, labels: []string{"key==val"}},
			want: &argRes{poolID: "122bc828", objID: "201e13ce", objPath: "test1"},
		},
		{
			name:        "pool/obj glob",
			uid:         "d87f7e0c",
			arg:         "test/test/test1/a...",
			fl:          &argFlags{},
			want:        &argRes{poolID: "122bc828", objPath: "test/test1/a"},
			wantOptsLen: 1,
		},
		{
			name:        "pool/obj glob and label",
			uid:         "d87f7e0c",
			arg:         "test/test/test1/a...",
			fl:          &argFlags{labels: []string{"key==val"}},
			want:        &argRes{poolID: "122bc828", objPath: "test/test1/a"},
			wantOptsLen: 2,
		},
		{
			name:        "pool/maybe obj  glob",
			uid:         "d87f7e0c",
			arg:         "test/test/test1/a...",
			fl:          &argFlags{poolObj: true},
			want:        &argRes{poolID: "122bc828", objPath: "test/test1/a"},
			wantOptsLen: 1,
		},
		{
			name:        "pool/maybe obj glob 2",
			uid:         "d87f7e0c",
			arg:         "test/test/test1/...",
			fl:          &argFlags{poolObj: true},
			want:        &argRes{poolID: "122bc828", objPath: "test/test1/"},
			wantOptsLen: 1,
		},
		{
			name:        "pool/obj full glob",
			uid:         "d87f7e0c",
			arg:         "test/...",
			fl:          &argFlags{},
			want:        &argRes{poolID: "122bc828"},
			wantOptsLen: 1,
		},
		{
			name:        "pool/obj full glob label",
			uid:         "d87f7e0c",
			arg:         "test/...",
			fl:          &argFlags{labels: []string{"key==val"}},
			want:        &argRes{poolID: "122bc828"},
			wantOptsLen: 2,
		},
		{
			name:        "pool/maybe obj full glob",
			uid:         "d87f7e0c",
			arg:         "test/...",
			fl:          &argFlags{poolObj: true},
			want:        &argRes{poolID: "122bc828"},
			wantOptsLen: 1,
		},
		{
			name: "copy user, cluster, pool and object",
			arg:  "user@cluster:pool/object",
			fl:   &argFlags{copy: true, poolObj: true},
			want: &argRes{dstUser: "user", cluster: "cluster", poolName: "pool", objPath: "object"},
		},
		{
			name: "copy user, cluster, pool",
			arg:  "user@cluster:pool",
			fl:   &argFlags{copy: true, poolObj: true},
			want: &argRes{dstUser: "user", cluster: "cluster", poolName: "pool"},
		},
		{
			name: "copy cluster, pool",
			arg:  "cluster:pool",
			fl:   &argFlags{copy: true, poolObj: true},
			want: &argRes{cluster: "cluster", poolName: "pool"},
		},
		{
			name: "copy user pool",
			arg:  "user@pool",
			fl:   &argFlags{copy: true, userAt: true, poolObj: true},
			want: &argRes{dstUser: "user", poolName: "pool"},
		},
		{
			name: "copy user pool object",
			arg:  "user@pool/object",
			fl:   &argFlags{copy: true, userAt: true, poolObj: true},
			want: &argRes{dstUser: "user", poolName: "pool", objPath: "object"},
		},
		{
			name: "copy user pool object",
			arg:  "user@pool/object",
			fl:   &argFlags{copy: true, userAt: true, poolObj: true},
			want: &argRes{dstUser: "user", poolName: "pool", objPath: "object"},
		},
		{
			name: "copy pool object",
			uid:  "d87f7e0c",
			arg:  "test/test1",
			fl:   &argFlags{copy: true, poolObj: true},
			want: &argRes{poolID: "122bc828", objID: "201e13ce", objPath: "test1"},
		},
		{
			name: "copy pool",
			uid:  "d87f7e0c",
			arg:  "test",
			fl:   &argFlags{copy: true, poolObj: true},
			want: &argRes{poolID: "122bc828"},
		},
		{
			name: "copy pool id",
			uid:  "d87f7e0c",
			arg:  "122bc828",
			fl:   &argFlags{id: true, copy: true, poolObj: true},
			want: &argRes{poolID: "122bc828"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArg(tt.uid, tt.arg, tt.fl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseArg() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(got, tt.want,
				cmp.AllowUnexported(argRes{}), cmpopts.IgnoreFields(argRes{}, "options")); diff != "" {
				t.Error(diff)
			}
			if got != nil && len(got.options) != tt.wantOptsLen {
				t.Errorf("expected opts length %d, got %d", tt.wantOptsLen, len(got.options))
			}
		})
	}
}
