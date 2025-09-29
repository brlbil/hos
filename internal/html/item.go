// SPDX-License-Identifier: MIT

package html

import (
	"path"
	"strings"
	"time"

	"github.com/brlbil/hos"
	"github.com/dustin/go-humanize"
)

type item struct {
	Name       string
	ModifiedAt string
	Size       string
}

func newItem(name string, modifiedAt time.Time, size int64) item {
	return item{
		Name:       name,
		ModifiedAt: formatTime(modifiedAt),
		Size:       humanize.Bytes(uint64(size)),
	}
}

func formatTime(t time.Time) string {
	if t.Equal(time.Time{}) {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func getItems[T hos.Pool | hos.Object](entities []T) []item {
	items := []item{}
	for _, entity := range entities {
		switch value := any(entity).(type) {
		case hos.Pool:
			items = append(items, newItem(value.Name+"/", value.ModifiedAt, value.Size))
		case hos.Object:
			name := path.Base(value.Name)
			if strings.HasSuffix(value.Name, "/") {
				name += "/"
			}
			items = append(items, newItem(name, value.ModifiedAt, value.Size))
		}
	}
	return items
}
