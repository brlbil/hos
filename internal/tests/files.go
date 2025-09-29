// SPDX-License-Identifier: MIT

package tests

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/brlbil/hos"
)

const (
	JPG = "test.jpg"
	AVI = "test.avi"
	CSV = "test.csv"

	contTypeJPG = "image/jpeg"
	contTypeAVI = "video/x-msvideo"
	contTypeCSV = "text/csv; charset=utf-8"
)

type File hos.Object

func (f *File) RelPath() string {
	up := ""
	for i := 0; ; i++ {
		if _, err := os.Stat(up + "go.mod"); err != nil {
			up += "../"
		} else {
			break
		}
		if i == 6 {
			panic(fmt.Sprintf("could not find go.mod file in %d iterations", i))
		}
	}
	dir := up + "internal/tests/files"
	switch f.ContentType {
	case contTypeJPG:
		return filepath.Join(dir, JPG)
	case contTypeAVI:
		return filepath.Join(dir, AVI)
	case contTypeCSV:
		return filepath.Join(dir, CSV)
	default:
		return filepath.Join(dir, f.Name)
	}
}

func (f *File) New(_ string) (io.ReadCloser, error) {
	file, err := os.Open(f.RelPath())
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (f *File) Copy(opts ...Option) *File {
	file := &File{
		Name:        f.Name,
		ContentType: f.ContentType,
		Size:        f.Size,
		Hash:        f.Hash,
	}

	for _, ofn := range opts {
		ofn(file)
	}

	return file
}

func (f *File) Obj(opts ...Option) *hos.Object {
	obj := &hos.Object{
		Name:         f.Name,
		PoolID:       f.PoolID,
		ContentType:  f.ContentType,
		Size:         f.Size,
		Hash:         f.Hash,
		Labels:       f.Labels,
		ReplicaCount: 1,
	}

	for _, ofn := range opts {
		ofn(obj)
	}

	return obj
}

var (
	FileJPG = File{
		Name:        "test.jpg",
		ContentType: contTypeJPG,
		Size:        1024,
		Hash:        "351358ca414e7f3d76f7ffdf8ed7bc00d7721143dc90fa4c80d5337c29a14048",
	}

	FileAVI = File{
		Name:        "test.avi",
		ContentType: contTypeAVI,
		Size:        2048,
		Hash:        "1bb34864b5cd45d408271a69f49e03c914e172f0a9624a63b4cdc672a7a93c3b",
	}

	FileCSV = File{
		Name:        "test.csv",
		ContentType: contTypeCSV,
		Size:        24,
		Hash:        "1901e7258e9dc51dd580973ec7e4998af6177906dbfaca6f12db6b6c03dd9deb",
	}
)
