// SPDX-License-Identifier: MIT

package dir

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestWalk(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantErr     string
		paths       []string
		want        []PathInfo
		recursive   bool
	}{
		{
			name:      "dir1, dir3",
			paths:     []string{"tests/dir1", "tests/dir2"},
			recursive: true,
			want: []PathInfo{
				{
					Name:    "tests/dir1/dir11/dir111/file111.txt",
					ConType: "text/plain; charset=utf-8",
				},
				{
					Name:    "tests/dir1/dir11/file11.data",
					ConType: "application/octet-stream",
					Size:    1024,
				},
				{
					Name:    "tests/dir1/dir12/file12.html",
					ConType: "text/html; charset=utf-8",
				},
				{
					Name:    "tests/dir2/file2.jpg",
					ConType: "image/jpeg",
				},
			},
		},
		{
			name:        "dir11, dir111",
			paths:       []string{"tests/dir1/dir11", "tests/dir1/dir11/dir111"},
			contentType: "app/mine",
			recursive:   true,
			want: []PathInfo{
				{
					Name:    "tests/dir1/dir11/dir111/file111.txt",
					ConType: "app/mine",
				},
				{
					Name:    "tests/dir1/dir11/file11.data",
					ConType: "app/mine",
					Size:    1024,
				},
			},
		},
		{
			name:      "not exist",
			paths:     []string{"tests/dir1/dir11", "tests/nope"},
			recursive: true,
			wantErr:   "lstat tests/nope: no such file or directory",
		},
		{
			name:  "all good",
			paths: []string{"tests/dir1/dir12/file12.html", "tests/dir2/file2.jpg"},
			want: []PathInfo{
				{Name: "file12.html", ConType: "text/html; charset=utf-8"},
				{Name: "file2.jpg", ConType: "image/jpeg"},
			},
		},
		{
			name:    "dir not allowed",
			paths:   []string{"tests/dir1/dir12/file12.html", "tests/dir2"},
			wantErr: "tests/dir2 is a directory, directories is not allowed in non recursive mod",
		},
		{
			name:    "not exist",
			paths:   []string{"tests/dir1/dir12/file13.html", "tests/dir2/file2.jpg"},
			wantErr: "stat tests/dir1/dir12/file13.html: no such file or directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Walk(tt.contentType, tt.recursive, tt.paths...)
			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("Walk() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreFields(PathInfo{}, "ReadCloser")); diff != "" {
				t.Error(diff)
			}
		})
	}
}
