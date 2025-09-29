// SPDX-License-Identifier: MIT

package tests

import (
	"fmt"
	"sync"

	"github.com/brlbil/hos"
)

type Port struct {
	mut sync.Mutex
	Val int
}

func (p *Port) Next() int {
	p.mut.Lock()
	p.Val++
	v := p.Val
	p.mut.Unlock()
	return v
}

func Map[T string | hos.Permission](a ...any) map[string]T {
	lena := len(a)
	if lena == 0 {
		return nil
	}

	m := map[string]T{}
	count := lena - (lena % 2)
	for i := 0; i < count; i += 2 {
		m[fmt.Sprint(a[i])] = a[i+1].(T)
	}
	return m
}
