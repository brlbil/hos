// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
	"github.com/brlbil/hos/internal/logger"
	"github.com/google/go-cmp/cmp"
)

type grandParent struct {
	ID   string `boltholdKey:"ID"`
	Name string
}

type parent struct {
	Labels  map[string]string
	ID      string `boltholdKey:"ID"`
	GrandID string `boltholdIndex:"GrandID"`
	Name    string
}

type child struct {
	Labels   map[string]string
	ID       string `boltholdKey:"ID"`
	ParentID string `boltholdIndex:"ParentID"`
	GrandID  string `boltholdIndex:"GrandID"`
	Name     string
}

func testDB(dir string, t *testing.T) *DB {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	log, err := logger.New("db-test", logger.WithLevel("none"))
	if err != nil {
		t.Fatal(err)
	}

	db, err := New(dir, log.Logger)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})

	return db
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n == 2 || n == 3 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}

	// Only check up to square root of n to optimize the check
	maxDivisor := int(math.Sqrt(float64(n)))
	for d := 5; d <= maxDivisor; d += 6 {
		if n%d == 0 || n%(d+2) == 0 {
			return false
		}
	}

	return true
}

func createRecords(db *DB, t *testing.T) {
	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	// ctx error
	if err := Create(cctx, db, &grandParent{}); err == nil {
		t.Fatalf("expected context error, got <il>")
	}

	// create a grand parent
	if err := Create(ctx, db, &grandParent{ID: "100", Name: "grandpa"}); err != nil {
		t.Fatal(err)
	}

	// create a some parents
	for i := 90; i < 92; i++ {
		id := fmt.Sprint(i)
		n := "odd"
		if i%2 == 0 {
			n = "even"
		}
		ll := map[string]string{"number": n}
		if err := Create(ctx, db, &parent{ID: id, GrandID: "100", Name: "parent" + id, Labels: ll}); err != nil {
			t.Fatal(err)
		}
	}

	// create a some child
	for i := 0; i <= 30; i++ {
		id := fmt.Sprint(i)
		n := "odd"
		p := "no"
		if i%2 == 0 {
			n = "even"
		}
		if isPrime(i) {
			p = "yes"
		}
		ll := map[string]string{"number": n, "prime": p}
		if err := Create(ctx, db, &child{ID: id, GrandID: "100", ParentID: "91", Name: "child" + id, Labels: ll}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreate(t *testing.T) {
	db := testDB(".create", t)

	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tests := []struct {
		ctx     context.Context
		t       *grandParent
		wantErr error
		name    string
	}{
		{
			name:    "canceled context",
			ctx:     cctx,
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "ok",
			ctx:  ctx,
			t:    &grandParent{ID: "1", Name: "lucky"},
		},
		{
			name:    "already exists",
			ctx:     ctx,
			t:       &grandParent{ID: "1", Name: "lucky"},
			wantErr: hos.ErrExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Create(tt.ctx, db, tt.t); !errors.Is(err, tt.wantErr) {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGet(t *testing.T) {
	db := testDB(".get", t)

	createRecords(db, t)

	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tests := []struct {
		ctx     context.Context
		want    *grandParent
		wantErr error
		name    string
		id      string
	}{
		{
			name:    "canceled context",
			ctx:     cctx,
			id:      "100",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "ok",
			ctx:  ctx,
			id:   "100",
			want: &grandParent{ID: "100", Name: "grandpa"},
		},
		{
			name:    "not exists",
			ctx:     ctx,
			id:      "101",
			wantErr: hos.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gp, err := Get[grandParent](tt.ctx, db, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(gp, tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	db := testDB(".update", t)

	createRecords(db, t)

	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tests := []struct {
		ctx     context.Context
		p       *parent
		want    *parent
		wantErr error
		name    string
	}{
		{
			name:    "canceled context",
			ctx:     cctx,
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "ok",
			ctx:  ctx,
			p:    &parent{ID: "90", GrandID: "100", Name: "90"},
			want: &parent{ID: "90", GrandID: "100", Name: "90"},
		},
		{
			name:    "not exists",
			ctx:     ctx,
			p:       &parent{ID: "999", GrandID: "100", Name: "90"},
			wantErr: hos.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Update(tt.ctx, db, tt.p); !errors.Is(err, tt.wantErr) {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}

			id := "100001"
			if tt.p != nil {
				id = tt.p.ID
			}
			// lets ignore the error
			gp, _ := Get[parent](ctx, db, id)

			if diff := cmp.Diff(gp, tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	db := testDB(".delete", t)

	createRecords(db, t)

	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tests := []struct {
		ctx        context.Context
		wantGetErr error
		wantErr    error
		name       string
		pid        string
		cid        string
	}{
		{
			name:    "canceled context",
			ctx:     cctx,
			pid:     "90",
			wantErr: context.DeadlineExceeded,
		},
		{
			name:       "del parent",
			ctx:        ctx,
			pid:        "90",
			wantGetErr: hos.ErrNotExist,
		},
		{
			name:       "del child",
			ctx:        ctx,
			cid:        "11",
			wantGetErr: hos.ErrNotExist,
		},
		{
			name:    "not empty parent",
			ctx:     ctx,
			pid:     "91",
			wantErr: hos.ErrNotEmpty,
		},
		{
			name:       "not exist parent",
			ctx:        ctx,
			pid:        "90",
			wantErr:    hos.ErrNotExist,
			wantGetErr: hos.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var getErr error
			if tt.pid != "" {
				if err := Delete[parent, child](tt.ctx, db, tt.pid); !errors.Is(err, tt.wantErr) {
					t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				}
				_, getErr = Get[parent](ctx, db, tt.pid)
			} else if tt.cid != "" {
				if err := Delete[child, child](tt.ctx, db, tt.cid); !errors.Is(err, tt.wantErr) {
					t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
				}
				_, getErr = Get[child](ctx, db, tt.cid)
			}

			if !errors.Is(getErr, tt.wantGetErr) {
				t.Errorf("Delete_Get() error = %v, wantErr %v", getErr, tt.wantGetErr)
			}
		})
	}
}

func TestList(t *testing.T) {
	db := testDB(".list", t)

	createRecords(db, t)

	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tests := []struct {
		ctx     context.Context
		want    any
		wantErr error
		name    string
		id      string
		kind    string
		qfs     []QueryFunc
	}{
		{
			name:    "canceled context",
			ctx:     cctx,
			kind:    "grandParent",
			want:    []grandParent{},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "list grand parents",
			ctx:  ctx,
			kind: "grandParent",
			want: []grandParent{
				{ID: "100", Name: "grandpa"},
			},
		},
		{
			name: "list parents, without parent id",
			ctx:  ctx,
			kind: "parent",
			want: []parent{},
		},
		{
			name: "list parents",
			ctx:  ctx,
			id:   "100",
			kind: "parent",
			want: []parent{
				{ID: "90", GrandID: "100", Name: "parent90", Labels: map[string]string{"number": "even"}},
				{ID: "91", GrandID: "100", Name: "parent91", Labels: map[string]string{"number": "odd"}},
			},
		},
		{
			name: "list 5 children from index 11",
			ctx:  ctx,
			id:   "91",
			kind: "child",
			qfs:  []QueryFunc{Range(11, 5)},
			want: []child{
				{ID: "19", ParentID: "91", GrandID: "100", Name: "child19", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "2", ParentID: "91", GrandID: "100", Name: "child2", Labels: map[string]string{"number": "even", "prime": "yes"}},
				{ID: "20", ParentID: "91", GrandID: "100", Name: "child20", Labels: map[string]string{"number": "even", "prime": "no"}},
				{ID: "21", ParentID: "91", GrandID: "100", Name: "child21", Labels: map[string]string{"number": "odd", "prime": "no"}},
				{ID: "22", ParentID: "91", GrandID: "100", Name: "child22", Labels: map[string]string{"number": "even", "prime": "no"}},
			},
		},
		{
			name: "list children with child3 name prefix",
			ctx:  ctx,
			id:   "91",
			kind: "child",
			qfs:  []QueryFunc{NamePrefix("child3")},
			want: []child{
				{ID: "3", ParentID: "91", GrandID: "100", Name: "child3", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "30", ParentID: "91", GrandID: "100", Name: "child30", Labels: map[string]string{"number": "even", "prime": "no"}},
			},
		},
		{
			name: "list first 2 children with child2 name prefix",
			ctx:  ctx,
			id:   "91",
			kind: "child",
			qfs:  []QueryFunc{NamePrefix("child2"), Range(0, 2)},
			want: []child{
				{ID: "2", ParentID: "91", GrandID: "100", Name: "child2", Labels: map[string]string{"number": "even", "prime": "yes"}},
				{ID: "20", ParentID: "91", GrandID: "100", Name: "child20", Labels: map[string]string{"number": "even", "prime": "no"}},
			},
		},
		{
			name: "list children with child1 name prefix and number even label",
			ctx:  ctx,
			id:   "91",
			kind: "child",
			qfs: []QueryFunc{
				NamePrefix("child1"),
				Labels([]filter.Label{
					{Key: "numer", Value: "even", Equal: false},
					{Key: "prime", Value: "yes", Equal: true},
					{Key: "nop", Value: "--", Equal: false},
				}...),
			},
			want: []child{
				{ID: "11", ParentID: "91", GrandID: "100", Name: "child11", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "13", ParentID: "91", GrandID: "100", Name: "child13", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "17", ParentID: "91", GrandID: "100", Name: "child17", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "19", ParentID: "91", GrandID: "100", Name: "child19", Labels: map[string]string{"number": "odd", "prime": "yes"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			var err error

			switch tt.kind {
			case "grandParent":
				got, err = List[grandParent](tt.ctx, db, tt.id, tt.qfs...)
			case "parent":
				got, err = List[parent](tt.ctx, db, tt.id, tt.qfs...)
			case "child":
				got, err = List[child](tt.ctx, db, tt.id, tt.qfs...)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestFind(t *testing.T) {
	db := testDB(".find", t)

	createRecords(db, t)

	ctx := context.Background()
	// deadline reached context
	cctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	tests := []struct {
		ctx     context.Context
		want    any
		wantErr error
		name    string
		s       string
		kind    string
	}{
		{
			name:    "canceled context",
			ctx:     cctx,
			kind:    "grandParent",
			want:    []grandParent{},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "list parents",
			ctx:  ctx,
			s:    "",
			kind: "parent",
			want: []parent{
				{ID: "90", GrandID: "100", Name: "parent90", Labels: map[string]string{"number": "even"}},
				{ID: "91", GrandID: "100", Name: "parent91", Labels: map[string]string{"number": "odd"}},
			},
		},
		{
			name: "list 5 children from index 11",
			ctx:  ctx,
			s:    "chl3",
			kind: "child",
			want: []child{
				{ID: "13", ParentID: "91", GrandID: "100", Name: "child13", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "23", ParentID: "91", GrandID: "100", Name: "child23", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "3", ParentID: "91", GrandID: "100", Name: "child3", Labels: map[string]string{"number": "odd", "prime": "yes"}},
				{ID: "30", ParentID: "91", GrandID: "100", Name: "child30", Labels: map[string]string{"number": "even", "prime": "no"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				got any
				err error
			)

			switch tt.kind {
			case "grandParent":
				got, err = Find[grandParent](tt.ctx, db, tt.s)
			case "parent":
				got, err = Find[parent](tt.ctx, db, tt.s)
			case "child":
				got, err = Find[child](tt.ctx, db, tt.s)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}
