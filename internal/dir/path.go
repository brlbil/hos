// SPDX-License-Identifier: MIT

package dir

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/brlbil/hos/internal/iofactory"
)

var getContentType func(string) (string, error)

type PathInfo struct {
	ReadCloser iofactory.ReadClosers
	Name       string
	ConType    string
	Size       int64
}

type pathMap struct {
	fileMap map[string]PathInfo
	dirMap  map[string]struct{}
}

func (p *pathMap) walkFunc(path string, dirEntry fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	if dirEntry.IsDir() {
		if _, exists := p.dirMap[path]; exists {
			// already walked this dir
			return filepath.SkipDir
		}
		p.dirMap[path] = struct{}{}
		return nil
	}

	fileInfo, infoErr := dirEntry.Info()
	if infoErr != nil {
		return infoErr
	}

	if strings.HasPrefix(fileInfo.Name(), ".") {
		return nil
	}

	return p.addFile(path, path, fileInfo.Size())
}

func (p *pathMap) read(paths ...string) error {
	for _, path := range paths {
		fileInfo, err := os.Stat(path)
		if err != nil {
			return err
		}

		if fileInfo.IsDir() {
			return fmt.Errorf("%s is a directory, directories is not allowed in non recursive mod", path)
		}

		if err := p.addFile(filepath.Base(path), path, fileInfo.Size()); err != nil {
			return err
		}
	}

	return nil
}

func (p *pathMap) addFile(name, path string, size int64) error {
	// lets get the absolute path for ReadClosers just in case
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// no need to check the file again, just ran os.Stat
	readCloser, err := iofactory.FileReadClosers(absolutePath)
	if err != nil {
		return err
	}

	contentType, err := getContentType(path)
	if err != nil {
		return err
	}

	p.fileMap[path] = PathInfo{
		Name:       strings.TrimPrefix(name, "/"),
		Size:       size,
		ReadCloser: readCloser,
		ConType:    contentType,
	}

	return nil
}

func (p *pathMap) pathInfo() []PathInfo {
	pathInfos := []PathInfo{}
	for _, pathInfo := range p.fileMap {
		pathInfos = append(pathInfos, pathInfo)
	}

	slices.SortFunc(pathInfos, func(path1, path2 PathInfo) int {
		if path1.Name < path2.Name {
			return -1
		}
		if path1.Name > path2.Name {
			return 1
		}
		return 0
	})

	return pathInfos
}

func readContentType(filename string) (string, error) {
	contentType := mime.TypeByExtension(filepath.Ext(filename))

	if contentType == "" {
		file, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("opening file %s failed: %w", filename, err)
		}
		defer file.Close()

		// read a chunk to decide between utf-8 text and binary
		var buffer [512]byte
		bytesRead, err := io.ReadFull(file, buffer[:])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return "", fmt.Errorf("reading from file %s failed: %w", filename, err)
		}

		contentType = http.DetectContentType(buffer[:bytesRead])
	}

	return contentType, nil
}
