// SPDX-License-Identifier: MIT

package fs

import (
	"io"
	"log/slog"
	"os"

	"github.com/minio/sio"
)

type encReadSeekCloser struct {
	file *os.File
	sr   *io.SectionReader
}

var _ io.ReadSeekCloser = &encReadSeekCloser{}

func (e *encReadSeekCloser) Read(buffer []byte) (int, error) {
	return e.sr.Read(buffer)
}

func (e *encReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	return e.sr.Seek(offset, whence)
}

func (e *encReadSeekCloser) Close() error {
	return e.file.Close()
}

func newEncReader(file *os.File, size int64, config *sio.Config, log *slog.Logger) (io.ReadSeekCloser, error) {
	if config == nil {
		log.Debug("no encryption config found")
		return file, nil
	}

	log.Debug("getting decryption reader")
	readerAt, err := sio.DecryptReaderAt(file, *config)
	if err != nil {
		return nil, err
	}
	sectionReader := io.NewSectionReader(readerAt, 0, size)

	return &encReadSeekCloser{file: file, sr: sectionReader}, nil
}
