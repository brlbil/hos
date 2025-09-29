// SPDX-License-Identifier: MIT

package header

import (
	"net/http"
	"testing"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var (
	sie  = &hos.ServerInfo{}
	siem = map[string]string{}

	si = &hos.ServerInfo{
		Statfs: hos.Statfs{
			BlockSize:       4096,
			Blocks:          2023,
			BlocksFree:      1024,
			BlocksAvailable: 2000,
			Inodes:          123456,
			InodesFree:      12345,
		},
		Operations: 2,
	}
	sim = map[string]string{
		"X-Hos-Disk-BlockSize":       "4096",
		"X-Hos-Disk-Blocks":          "2023",
		"X-Hos-Disk-BlocksAvailable": "2000",
		"X-Hos-Disk-BlocksFree":      "1024",
		"X-Hos-Disk-Inodes":          "123456",
		"X-Hos-Disk-InodesFree":      "12345",
		"X-Hos-Operations-Count":     "2",
	}

	pe  = &hos.Pool{}
	pem = map[string]string{
		"X-Hos-Pool-Id":       "",
		"X-Hos-Pool-Name":     "",
		"X-Hos-Replica-Count": "0",
		"X-Hos-User-Id":       "",
	}

	p = &hos.Pool{
		Name:         "pool1",
		ID:           "238DHT7",
		UserID:       "8EJD4FTJ",
		ReplicaCount: 3,
		LinkedID:     "UDJ7R8DJ",
		ObjectCount:  2,
		Size:         2023,
		Encrypted:    true,
		CreatedAt:    time.Date(2015, 10, 21, 0o7, 28, 0, 0, time.UTC),
		ModifiedAt:   time.Date(2015, 10, 21, 0o7, 28, 0, 0, time.UTC),
		Hash:         "sda7hsd8shd6hds3sdd",
		Labels:       map[string]string{"K": "V"},
		Permissions:  map[string]hos.Permission{"*": "r"},
		Attributes:   map[string]string{"Replication": `{"Address":"example.com:8178"}`},
	}
	pm = map[string]string{
		"X-Hos-Pool-Id":         "238DHT7",
		"X-Hos-Pool-Name":       "pool1",
		"X-Hos-Replica-Count":   "3",
		"X-Hos-User-Id":         "8EJD4FTJ",
		"X-Hos-Labels":          `"K"="V"`,
		"X-Hos-Permissions":     `"*"="r"`,
		"X-Hos-Attributes":      `"Replication"="{"Address":"example.com:8178"}"`,
		"X-Hos-Linked-Id":       "UDJ7R8DJ",
		"X-Hos-Object-Count":    "2",
		"X-Hos-Encrypted":       "true",
		"X-Hos-Pool-Bytes-Used": "2023",
		"ETag":                  "sda7hsd8shd6hds3sdd",
		"Last-Modified":         "Wed, 21 Oct 2015 07:28:00 UTC",
		"X-Hos-Created":         "Wed, 21 Oct 2015 07:28:00 UTC",
	}

	oe  = &hos.Object{}
	oem = map[string]string{
		"Content-Type":         "",
		"X-Hos-Content-Length": "0",
		"X-Hos-Object-Id":      "",
		"X-Hos-Object-Name":    "",
		"X-Hos-Pool-Id":        "",
		"X-Hos-Replica-Count":  "0",
		"X-Hos-User-Id":        "",
	}

	o = &hos.Object{
		Name:         "obj1",
		ID:           "IJ7DH9TJ",
		PoolID:       "238DHT7",
		UserID:       "8EJD4FTJ",
		ReplicaCount: 2,
		ContentType:  "text/csv",
		Size:         13,
		Encrypted:    true,
		CreatedAt:    time.Date(2015, 10, 21, 0o7, 28, 0, 0, time.UTC),
		ModifiedAt:   time.Date(2015, 10, 21, 0o7, 28, 0, 0, time.UTC),
		Hash:         "kwh983hw834jhf8df",
		Labels:       map[string]string{"K": "V"},
	}
	om = map[string]string{
		"Content-Type":         "text/csv",
		"ETag":                 "kwh983hw834jhf8df",
		"X-Hos-Content-Length": "13",
		"X-Hos-Object-Id":      "IJ7DH9TJ",
		"X-Hos-Object-Name":    "obj1",
		"X-Hos-Pool-Id":        "238DHT7",
		"X-Hos-Replica-Count":  "2",
		"X-Hos-Encrypted":      "true",
		"X-Hos-User-Id":        "8EJD4FTJ",
		"X-Hos-Labels":         `"K"="V"`,
		"Last-Modified":        "Wed, 21 Oct 2015 07:28:00 UTC",
		"X-Hos-Created":        "Wed, 21 Oct 2015 07:28:00 UTC",
	}

	fle = &filter.Headers{}

	fl = &filter.Headers{
		Range:      []int{0, 50},
		NamePrefix: "S",
		Labels: []filter.Label{
			{Key: "K", Value: "V", Equal: true},
			{Key: "T", Value: "X", Equal: false},
		},
	}
	flm = map[string]string{
		"X-Hos-Range-Filter":       "0,50",
		"X-Hos-Name-Prefix-Filter": "S",
		"X-Hos-Label-Filters":      "W3siS2V5IjoiSyIsIlZhbHVlIjoiViIsIkVxdWFsIjp0cnVlfSx7IktleSI6IlQiLCJWYWx1ZSI6IlgiLCJFcXVhbCI6ZmFsc2V9XQ==",
	}
)

func TestSerialize(t *testing.T) {
	tests := []struct {
		a    any
		want map[string]string
		name string
	}{
		{name: "server info empty", a: sie, want: siem},
		{name: "server info", a: si, want: sim},
		{name: "pool empty", a: pe, want: pem},
		{name: "pool", a: p, want: pm},
		{name: "object empty", a: oe, want: oem},
		{name: "object", a: o, want: om},
		{name: "options", a: fle, want: map[string]string{}},
		{name: "options", a: fl, want: flm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Serialize(tt.a)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		header  map[string]string
		want    any
		wantErr string
	}{
		{name: "server info empty", header: siem, want: sie},
		{name: "server info", header: sim, want: si},
		{name: "pool empty", header: pem, want: pe},
		{name: "pool", header: pm, want: p},
		{name: "object empty", header: oem, want: oe},
		{name: "object", header: om, want: o},
		{name: "filter empty", header: map[string]string{}, want: fle},
		{name: "filter", header: flm, want: fl},
		{
			name: "wrong int", header: map[string]string{"X-Hos-Replica-Count": "a"}, want: &hos.Pool{},
			wantErr: `parsing header X-Hos-Replica-Count failed strconv.ParseInt: parsing "a": invalid syntax, bad request`,
		},
		{
			name: "wrong time", header: map[string]string{"Last-Modified": "not a time value"}, want: &hos.Object{},
			wantErr: `parsing header Last-Modified failed parsing time "not a time value" as "Mon, 02 Jan 2006 15:04:05 MST": cannot parse "not a time value" as "Mon", bad request`,
		},
		{
			name: "wrong map - label", header: map[string]string{"X-Hos-Labels": `"K";`}, want: &hos.Object{},
			wantErr: `wrong value "K";, expected format key=val bad request`,
		},
		{
			name: "wrong key - label", header: map[string]string{"X-Hos-Labels": `"1K"="1"`}, want: &hos.Object{},
			wantErr: `key 1K is not valid bad request`,
		},
		{
			name: "invalid map - perm", header: map[string]string{"X-Hos-Permissions": `"*"="d"`}, want: &hos.Pool{},
			wantErr: "d is not a valid permission, must be one of 'r' or 'w'\nbad request",
		},
		{
			name: "wrong key - perm", header: map[string]string{"X-Hos-Permissions": `"1K"="r"`}, want: &hos.Pool{},
			wantErr: `1K is not a valid permission selector, must be either * or a user name,
user name must start with a letter, only contain letters or numbers, not be longer than 10 characters
bad request`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			var err error

			switch tt.want.(type) {
			case *hos.ServerInfo:
				got, err = Parse[hos.ServerInfo](toHeader(tt.header))
			case *hos.Pool:
				got, err = Parse[hos.Pool](toHeader(tt.header))
			case *hos.Object:
				got, err = Parse[hos.Object](toHeader(tt.header))
			case *filter.Headers:
				got, err = Parse[filter.Headers](toHeader(tt.header))
			}

			if err != nil && err.Error() != tt.wantErr {
				t.Errorf("expected error is not equal to error got\n%s", cmp.Diff(tt.wantErr, err.Error()))
			}
			if tt.wantErr != "" {
				return
			}

			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(hos.Object{})); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func toHeader(m map[string]string) http.Header {
	h := http.Header{}
	for k, v := range m {
		h.Set(k, v)
	}
	return h
}
