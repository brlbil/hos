// SPDX-License-Identifier: MIT

package client

import (
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/compare"
	"github.com/brlbil/hos/pkg/id"
)

// responseFilter interface implements method to filter returned objects
type responseFilters interface {
	Filter(any) any
}

func filters(opts ...Options) []responseFilters {
	fts := []responseFilters{}
	for _, opt := range opts {
		m, ok := opt.(responseFilters)
		if ok {
			fts = append(fts, m)
		}
	}
	return fts
}

type fieldEqual struct {
	fieldName string
	values    []string
}

func (*fieldEqual) Option() {}

func (fe *fieldEqual) Filter(a any) any {
	val := reflect.ValueOf(a)
	if a == nil || val.Kind() != reflect.Slice || val.Len() == 0 {
		return a
	}

	retSlice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(val.Index(0).Interface())), 0, 0)
	for i := 0; i < val.Len(); i++ {
		f := val.Index(i)
		fv := f.FieldByName(fe.fieldName)
		if slices.Contains(fe.values, fv.String()) {
			retSlice = reflect.Append(retSlice, f)
		}
	}

	return retSlice.Interface()
}

func FilterByField(fieldName string, values ...string) *fieldEqual {
	return &fieldEqual{fieldName: fieldName, values: values}
}

type dirListing struct {
	objIDMap map[string]int
	parent   string
}

func (*dirListing) Option() {}

func (d *dirListing) Filter(a any) any {
	objects, ok := a.([]hos.Object)
	if !ok {
		return a
	}

	oo := []hos.Object{}
	for _, o := range objects {
		trimmedPrefix := strings.TrimPrefix(o.Name, d.parent)
		// name does not have the prefix so filter it
		if len(d.parent) > 0 && trimmedPrefix == o.Name {
			continue
		}

		paths := strings.Split(trimmedPrefix, "/")
		index := 0
		if paths[0] == "" && len(paths) > 1 {
			index = 1
		}

		name := o.Name
		oid := o.ID
		if index+1 != len(paths) {
			name = path.Join(d.parent, paths[index])
			oid = id.Gen(o.PoolID, name)
		}

		if index, ok := d.objIDMap[oid]; ok {
			b4o := oo[index]
			b4o.Size += o.Size
			if b4o.CreatedAt.After(o.CreatedAt) {
				b4o.CreatedAt = o.CreatedAt
			}
			if b4o.ModifiedAt.Before(o.ModifiedAt) {
				b4o.ModifiedAt = o.ModifiedAt
			}
			b4o.Hash = calHash(b4o.Hash, o.Hash)

			oo[index] = b4o
			continue
		}

		if o.Name != name {
			o.Name = name + "/"
			o.ContentType = "application/hos-dir+json"
			o.ID = oid
		}

		oo = append(oo, o)
		d.objIDMap[oid] = len(oo) - 1
	}

	return oo
}

// ObjectDirectoryListing returns ResponseFilter that sets which filters and modifies objects
// for easier directory listing
func ObjectDirectoryListing(prefix string) *dirListing {
	return &dirListing{parent: prefix, objIDMap: map[string]int{}}
}

type poolCorrector struct{}

func (*poolCorrector) Filter(a any) any {
	pools, ok := a.([]hos.Pool)
	if !ok {
		return a
	}
	for i, p := range pools {
		if p.ReplicaCount == 0 {
			continue
		}
		p.ObjectCount /= p.ReplicaCount
		p.Size /= int64(p.ReplicaCount)
		pools[i] = p
	}

	return pools
}

type sortByName struct{}

func (*sortByName) Filter(a any) any {
	switch v := a.(type) {
	case []hos.Pool:
		slices.SortFunc(v, compare.Pool)
		return v
	case []hos.Object:
		slices.SortFunc(v, compare.Object)
		return v
	}
	return a
}

type serverAddrFilter struct {
	hostMap map[string]string
}

func (serverAddrFilter) Option() {}

func (s *serverAddrFilter) Filter(a any) any {
	objects, ok := a.([]hos.Object)
	if !ok {
		return a
	}

	for i := range objects {
		addr := s.hostMap[objects[i].ID]
		objects[i].SetServerAddr(addr)
	}

	return objects
}
