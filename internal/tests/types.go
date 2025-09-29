// SPDX-License-Identifier: MIT

package tests

import (
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/pkg/id"
)

type Option func(entity any)

func Attrs(attrs ...any) Option {
	return func(entity any) {
		switch pool := entity.(type) {
		case *hos.Pool:
			pool.Attributes = Map[string](attrs...)
		}
	}
}

func CreatedAt(timestamp time.Time) Option {
	return func(entity any) {
		switch file := entity.(type) {
		case *File:
			file.CreatedAt = timestamp
		}
	}
}

func ContentType(contentType string) Option {
	return func(entity any) {
		switch item := entity.(type) {
		case *hos.Object:
			item.ContentType = contentType
		case *File:
			item.ContentType = contentType
		}
	}
}

func Encrypted() Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.Encrypted = true
		case *hos.Object:
			i.Encrypted = true
		}
	}
}

func Hash(h string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.Hash = h
		case *hos.Object:
			i.Hash = h
		case *File:
			i.Hash = h
		}
	}
}

func ID(id string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.ID = id
		case *hos.Object:
			i.ID = id
		}
	}
}

func Labels(l ...any) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.Labels = Map[string](l...)
		case *hos.Object:
			i.Labels = Map[string](l...)
		case *File:
			i.Labels = Map[string](l...)
		}
	}
}

func Linked(uid, pool string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.LinkedID = id.Gen(uid, pool)
		}
	}
}

func Name(n string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Object:
			i.Name = n
			i.ID = id.Gen(i.PoolID, n)
		case *File:
			i.Name = n
		}
	}
}

func ObjCount(c int) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.ObjectCount = c
		}
	}
}

func Perms(p ...any) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.Permissions = Map[hos.Permission](p...)
		}
	}
}

func PoolID(uid, pool string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *File:
			i.PoolID = PID(uid, pool)
		}
	}
}

func RepCount(rc int) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.ReplicaCount = rc
		case *hos.Object:
			i.ReplicaCount = rc
		case *File:
			i.ReplicaCount = rc
		}
	}
}

func UserID(uid string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Pool:
			i.UserID = uid
			i.ID = PID(uid, i.Name)
		case *hos.Object:
			i.UserID = uid
		}
	}
}

func UserPoolID(uid, pool string) Option {
	return func(a any) {
		switch i := a.(type) {
		case *hos.Object:
			i.UserID = uid
			i.PoolID = id.Gen(uid, pool)
			i.ID = id.Gen(i.PoolID, i.Name)
		case *File:
			i.UserID = uid
			i.PoolID = id.Gen(uid, pool)
		}
	}
}

func Size(i int64) Option {
	return func(a any) {
		switch b := a.(type) {
		case *hos.Pool:
			b.Size = i
		case *hos.Object:
			b.Size = i
		case *File:
			b.Size = i
		}
	}
}

func Object(name string, opts ...Option) *hos.Object {
	object := &hos.Object{Name: name}

	for _, optionFunc := range opts {
		optionFunc(object)
	}

	return object
}

func Pool(name string, opts ...Option) *hos.Pool {
	pool := &hos.Pool{Name: name}

	for _, optionFunc := range opts {
		optionFunc(pool)
	}

	return pool
}
