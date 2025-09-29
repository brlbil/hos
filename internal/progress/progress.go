// SPDX-License-Identifier: MIT

// Package progress provides terminal progress bar functionality for HOS operations.
// It wraps mpb to display upload/download progress with custom styling.
package progress

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/brlbil/hos/internal/iofactory"
	"github.com/muesli/termenv"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Progress represents a progress bar for io operations
type Progress struct {
	progressBar *mpb.Progress
	name        string
	size        int64
	count       int
	output      *termenv.Output
}

// Wait waits for all progress bars to complete
func (p *Progress) Wait() {
	p.count = p.progressBar.BarCount()
	p.progressBar.Wait()
}

// Remove clears progress bars from terminal display
func (p *Progress) Remove() {
	count := p.count + 1
	for range count {
		fmt.Print("\r\033[2K")
		// move up one line
		fmt.Print("\033[1A")
	}
}

const (
	check      = "✓"
	checkColor = "#15C200"
	arrowColor = "#D12626"
)

// Add creates a new progress bar for a server connection and wraps the reader
func (p *Progress) Add(server string, reader io.ReadCloser) io.ReadCloser {
	serverName := " " + server

	checkMark := p.output.String(check).Foreground(p.output.Color(checkColor)).String()

	arrowSymbol := "<"
	if _, ok := reader.(*os.File); ok {
		arrowSymbol = ">"
	}
	arrow := p.output.String(arrowSymbol).Foreground(p.output.Color(arrowColor)).String()

	barStyle := mpb.BarStyle()
	barStyle.Lbound("").Filler("█").Tip("█").Padding("░").Rbound("")
	bar := p.progressBar.New(
		p.size,
		barStyle,
		mpb.PrependDecorators(
			decor.Name(serverName, decor.WC{W: len(serverName), C: decor.DidentRight | decor.DextraSpace}, decor.WCSyncSpaceR),
			decor.OnComplete(decor.Name(arrow, decor.WCSyncSpaceR), ""),
			decor.OnComplete(decor.CountersKibiByte("% .2f / % .2f"), ""),
		),
		mpb.AppendDecorators(
			decor.NewPercentage("%d", decor.WCSyncSpaceR),
			decor.OnComplete(decor.AverageETA(decor.ET_STYLE_GO, decor.WCSyncSpaceR), ""),
			decor.OnComplete(decor.AverageSpeed(decor.UnitKiB, "% .2f"), ""),
		),
		mpb.BarFillerOnComplete(checkMark),
	)

	barReadCloser := &barReadCloser{readCloser: bar.ProxyReader(reader), bar: bar}
	return barReadCloser
}

// New creates a new progress bar container for an object with given size
func New(name string, size int64) *Progress {
	output := termenv.NewOutput(os.Stdout)
	progressBar := mpb.New(
		mpb.WithWidth(35),
		mpb.WithRefreshRate(50*time.Millisecond),
	)

	fmt.Println(name)

	return &Progress{name: name, size: size, progressBar: progressBar, output: output}
}

type progressReadClosers struct {
	prog        *Progress
	readClosers iofactory.ReadClosers
}

// ProgressReadClosers wraps ReadClosers with progress tracking
func ProgressReadClosers(readClosers iofactory.ReadClosers, prog *Progress) iofactory.ReadClosers {
	return &progressReadClosers{prog: prog, readClosers: readClosers}
}

// New creates a new ReadCloser with progress tracking for the given server
func (progressReader *progressReadClosers) New(server string) (io.ReadCloser, error) {
	readCloser, err := progressReader.readClosers.New(server)
	if err != nil {
		return nil, err
	}
	return progressReader.prog.Add(server, readCloser), nil
}

type barReadCloser struct {
	readCloser io.ReadCloser
	bar        *mpb.Bar
}

// Read implements io.Reader with progress tracking
func (barReader *barReadCloser) Read(buffer []byte) (int, error) {
	return barReader.readCloser.Read(buffer)
}

// Close implements io.Closer and aborts the progress bar
func (barReader *barReadCloser) Close() error {
	err := barReader.readCloser.Close()
	barReader.bar.Abort(true)
	barReader.bar.Wait()
	return err
}
