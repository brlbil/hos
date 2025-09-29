// SPDX-License-Identifier: MIT

// Package filter provides query filtering structures for HOS API requests.
// It handles range, name prefix, and label-based filtering via HTTP headers.
package filter

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Label represents a key-value label filter with equality comparison
type Label struct {
	Key   string
	Value string
	Equal bool
}

// Headers represents filtering options passed via HTTP headers
type Headers struct {
	Range      []int   `header:"X-Hos-Range-Filter,omitempty"`
	NamePrefix string  `header:"X-Hos-Name-Prefix-Filter,omitempty"`
	Labels     []Label `header:"X-Hos-Label-Filters,omitempty"`
}

// filter represents a single filter parameter
type filter struct {
	key string
	val string
}

// Option marks filter as a client option
func (*filter) Option() {}

// ModifyRequest adds filter headers to HTTP requests
func (f *filter) ModifyRequest(request *http.Request) error {
	if f.key == "" {
		return errors.New("filer key cannot be empty")
	}
	request.Header.Set(f.key, f.val)
	return nil
}

// Range creates a range filter
func Range(start, end uint) *filter {
	return &filter{key: "X-Hos-Range-Filter", val: fmt.Sprintf("%d,%d", start, end)}
}

// NamePrefix creates a name prefix filter
func NamePrefix(prefix string) *filter {
	return &filter{key: "X-Hos-Name-Prefix-Filter", val: prefix}
}

// Labels creates a label-based filter
func Labels(labels []Label) *filter {
	jsonBytes, err := json.Marshal(&labels)
	if err != nil {
		return &filter{}
	}
	encodedValue := base64.StdEncoding.EncodeToString(jsonBytes)
	return &filter{key: "X-Hos-Label-Filters", val: encodedValue}
}
