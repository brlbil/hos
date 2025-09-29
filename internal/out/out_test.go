// SPDX-License-Identifier: MIT

package out

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type customB []byte

func (cb customB) String() string {
	if len(cb) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(cb)
}

type ts struct {
	Name       string            `json:"name" yaml:"name" print:"default,wide"`
	Number     int               `json:"number,omitempty" yaml:"number" print:"default,wide"`
	LongNumber int64             `json:"long_number,omitempty" yaml:"longNumber" print:"default,wide"`
	Text1      string            `json:"text1" yaml:"text1" print:"wide"`
	Text2      string            `json:"text2,omitempty" yaml:"text2,omitempty"`
	Bool       bool              `json:"bool,omitempty" yaml:"bool,omitempty" print:"wide"`
	TimeOne    time.Time         `json:"time_one,omitempty" yaml:"timeOne,omitempty"`
	TimeTwo    time.Time         `json:"time_two,omitempty" yaml:"timeTwo,omitempty" print:"wide"`
	Slice      []customB         `json:"slice,omitempty" yaml:"slice,omitempty" print:"wide"`
	SSlice     []string          `json:"sslice,omitempty" yaml:"sslice,omitempty" print:"wide"`
	Map        map[string]string `json:"map,omitempty" yaml:"map,omitempty"`
}

func Test_print(t *testing.T) {
	testStructPointers := []*ts{
		{},
		{
			Name:       "test1",
			Number:     1,
			LongNumber: 1223334444,
			Text1:      "text11",
			Text2:      "text21",
			TimeOne:    time.Now().Add(-time.Hour * 1),
			TimeTwo:    time.Now().Add(-time.Hour * 2),
			Slice:      []customB{{23, 43, 77, 89, 84}},
			SSlice:     []string{"xxx"},
			Map:        map[string]string{},
		},
		{
			Name:       "test2",
			Number:     2,
			LongNumber: 22333444455555,
			Text1:      "text21",
			Text2:      "text22",
			Bool:       true,
			TimeOne:    time.Now().Add(-time.Hour * 1),
			TimeTwo:    time.Now().Add(-time.Hour * 2),
			Slice:      []customB{{23, 43, 77, 89}, {98, 56, 34, 67, 33}},
			SSlice:     []string{"yyy", "zzz"},
			Map:        map[string]string{},
		},
	}

	testStruct := []ts{*testStructPointers[1], *testStructPointers[2]}

	tests := []struct {
		name    string
		a       any
		ot      string
		want    string
		wantErr bool
	}{
		{
			name: "default+nil",
			a:    nil,
			ot:   "default",
		},
		{
			name: "default+empty+slice",
			a:    []ts{},
			ot:   "default",
		},
		{
			name: "default+empty",
			a:    testStructPointers[0],
			ot:   "default",
			want: "NAME\tNUMBER\t\tLONGNUMBER\n\t0\t\t0 B\n",
		},
		{
			name: "default+one",
			a:    testStructPointers[1],
			ot:   "default",
			want: "NAME\t\tNUMBER\t\tLONGNUMBER\ntest1\t\t1\t\t1.2 GB\n",
		},
		{
			name: "default+one+struct",
			a:    testStruct[0],
			ot:   "default",
			want: "NAME\t\tNUMBER\t\tLONGNUMBER\ntest1\t\t1\t\t1.2 GB\n",
		},
		{
			name: "default+one+two",
			a:    testStructPointers[1:],
			ot:   "default",
			want: "NAME\t\tNUMBER\t\tLONGNUMBER\ntest1\t\t1\t\t1.2 GB\ntest2\t\t2\t\t22 TB\n",
		},
		{
			name: "default+one+two+struct",
			a:    testStruct,
			ot:   "default",
			want: "NAME\t\tNUMBER\t\tLONGNUMBER\ntest1\t\t1\t\t1.2 GB\ntest2\t\t2\t\t22 TB\n",
		},
		{
			name: "wide+one",
			a:    testStructPointers[1],
			ot:   "wide",
			want: "NAME\t\tNUMBER\t\tLONGNUMBER\tTEXT1\t\tBOOL\tTIMETWO\t\tSLICE\t\tSSLICE\ntest1\t\t1\t\t1.2 GB\t\ttext11\t\tno\t2 hours ago\tFytNWVQ=\txxx\n",
		},
		{
			name: "wide+one+two",
			a:    testStructPointers[1:],
			ot:   "wide",
			want: "NAME\t\tNUMBER\t\tLONGNUMBER\tTEXT1\t\tBOOL\tTIMETWO\t\tSLICE\t\t\tSSLICE\ntest1\t\t1\t\t1.2 GB\t\ttext11\t\tno\t2 hours ago\tFytNWVQ=\t\txxx\ntest2\t\t2\t\t22 TB\t\ttext21\t\tyes\t2 hours ago\tFytNWQ== YjgiQyE=\tyyy zzz\n",
		},
		{
			name: "name+one",
			a:    testStructPointers[1],
			ot:   "name",
			want: "NAME\ntest1\n",
		},
		{
			name: "default+one+two",
			a:    testStructPointers[1:],
			ot:   "name",
			want: "NAME\ntest1\ntest2\n",
		},
		{
			name: "fields+one",
			a:    testStructPointers[1],
			ot:   "fields=Name,text2,timeOne",
			want: "NAME\t\tTEXT2\t\tTIMEONE\ntest1\t\ttext21\t\t1 hour ago\n",
		},
		{
			name: "fields+one+two",
			a:    testStructPointers[1:],
			ot:   "fields=Name,text2,timeOne",
			want: "NAME\t\tTEXT2\t\tTIMEONE\ntest1\t\ttext21\t\t1 hour ago\ntest2\t\ttext22\t\t1 hour ago\n",
		},
		{
			name: "json",
			a:    testStructPointers[0],
			ot:   "json",
			want: "{\n    \"name\": \"\",\n    \"text1\": \"\",\n    \"time_one\": \"0001-01-01T00:00:00Z\",\n    \"time_two\": \"0001-01-01T00:00:00Z\"\n}\n",
		},
		{
			name: "yaml",
			a:    testStructPointers[0],
			ot:   "yaml",
			want: "name: \"\"\nnumber: 0\nlongNumber: 0\ntext1: \"\"\n",
		},
	}

	buf := &bytes.Buffer{}
	defWriter = buf

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer buf.Reset()

			if err := Print(tt.a, tt.ot); (err != nil) != tt.wantErr {
				t.Errorf("print() error = %v, wantErr %v", err, tt.wantErr)
			}

			// check written value
			if diff := cmp.Diff(buf.String(), tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}
