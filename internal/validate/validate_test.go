// SPDX-License-Identifier: MIT

package validate

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
	"github.com/google/go-cmp/cmp"
)

func TestPool(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{name: "match", s: "Test_1"},
		{name: "match -", s: "test-1"},
		{name: "match max chars", s: "test_with_some_amount_123"},
		{name: "match double", s: "t1"},
		{name: "not match start number", s: "1test", wantErr: true},
		{name: "not match char", s: "test+1", wantErr: true},
		{name: "not matching end char", s: "test_", wantErr: true},
		{name: "not matching too long", s: "test_with_some_amount_1234", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Pool(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("Pool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestObject(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{name: "match", s: "some/path/with pretty permissive chars/(;)[@#$%^&].to"},
		{name: "match short", s: "test/@%"},
		{name: "match long", s: "a" + strings.Repeat("b", 1022) + "c"},
		{name: "not match illegal char \\", s: "test/a\\b", wantErr: true},
		{name: "not match start with /", s: "/test/ab", wantErr: true},
		{name: "not match ends with space", s: "test/ab ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Object(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("Object() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "match lower case", id: "1a47f8c3"},
		{name: "match upper case", id: "1A47F8C3"},
		{name: "not match not allowed char", id: "1g47f8c3", wantErr: true},
		{name: "not match longer than 8", id: "1a47f8c37", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ID(tt.id); (err != nil) != tt.wantErr {
				t.Errorf("ID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{name: "match", s: "Test/test_1"},
		{name: "match -", s: "test-1 4"},
		{name: "match short", s: "t"},
		{name: "match max char", s: "test_with_some_amount_123"},
		{name: "not match start number", s: "1test", wantErr: true},
		{name: "not match char", s: "test+1", wantErr: true},
		{name: "not matching end char", s: "test_", wantErr: true},
		{name: "not matching too long", s: "test_with_some_amount_1234", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Label(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("Label() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLabelValue(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{name: "match", s: "Test/test_1"},
		{name: "match chars", s: "test-1 /-_,.;:4"},
		{name: "match short", s: "t"},
		{name: "not match start number", s: "1test", wantErr: true},
		{name: "not match char", s: "test+1", wantErr: true},
		{name: "not matching end char", s: "test_", wantErr: true},
		{name: "not matching too long", s: strings.Repeat("s", 256), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := LabelValue(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("Label() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseLabel(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    string
		want1   string
		wantErr bool
	}{
		{
			name:  "match",
			s:     "label=This is label with all the allowed chars / - _ , . ; : E",
			want:  "label",
			want1: "This is label with all the allowed chars / - _ , . ; : E",
		},
		{
			name:  "match short",
			s:     "l=t",
			want:  "l",
			want1: "t",
		},
		{
			name:    "not match, not allowed char *",
			s:       "label=This is label with all the allowed chars / - _ * , . ; : E",
			wantErr: true,
		},
		{
			name:    "not match, too long",
			s:       "label=This is label with all the allowed chars  but too long   / - _ * , . ; : E",
			wantErr: true,
		},
		{
			name:    "not match, wrong label",
			s:       "1label=This is label 1",
			wantErr: true,
		},
		{
			name:    "not match, not have =",
			s:       "1labelThis is label 1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParseLabel(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLabel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(got1, tt.want1); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestParseAttr(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    string
		want1   string
		wantErr bool
	}{
		{
			name:  "match",
			s:     "attr=It is hard to find not matching char {} () [] ;'./\\ 49 ,~",
			want:  "attr",
			want1: "It is hard to find not matching char {} () [] ;'./\\ 49 ,~",
		},
		{
			name:    "not match, not allowed char *",
			s:       "attr=This is attr with not allowed char å",
			wantErr: true,
		},
		{
			name:    "not match, too long",
			s:       "attr=" + strings.Repeat("-", 1001),
			wantErr: true,
		},
		{
			name:    "not match, wrong attr",
			s:       "1attr=This is attr 1",
			wantErr: true,
		},
		{
			name:    "not match, not have =",
			s:       "1attr is attr 1",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParseAttr(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAttr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(got1, tt.want1); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestUserCluster(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{name: "match", s: "Test1"},
		{name: "not match start number", s: "1test", wantErr: true},
		{name: "not match char", s: "test+1", wantErr: true},
		{name: "not matching too long", s: "test1234569u", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := User(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("User() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := Cluster(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("Cluster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPermSelector(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{name: "match *", s: "*"},
		{name: "match user", s: "user1"},
		{name: "not match", s: "user*1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PermSelector(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("PermSelector() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePerm(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    string
		perm    hos.Permission
		wantErr bool
	}{
		{name: "match * read", s: "*:r", want: "*", perm: read},
		{name: "match * write", s: "*:w", want: "*", perm: write},
		{name: "match user read", s: "user1:r", want: "user1", perm: read},
		{name: "match user write", s: "user1:w", want: "user1", perm: write},
		{name: "not match *user", s: "*u:w", wantErr: true},
		{name: "not match user", s: "u_1:r", wantErr: true},
		{name: "not match perm", s: "u1:z", wantErr: true},
		{name: "not match no :", s: "user1x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, perm, err := ParsePerm(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePerm() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err := Perm(perm); err != nil && !tt.wantErr {
				t.Errorf("Expected perm, got err %s", err)
			}

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(perm, tt.perm); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestAddress(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{name: "valid ip", ip: "10.1.1.2"},
		{name: "valid name", ip: "example.com"},
		{name: "valid ip port", ip: "10.1.1.2:1024"},
		{name: "valid name port", ip: "example.com:1024"},
		{name: "not valid ip", ip: "10.1.1.355", wantErr: true},
		{name: "not valid ip port", ip: "10.1.1.1:w", wantErr: true},
		{name: "not valid domain", ip: "example*com", wantErr: true},
		{name: "not valid domain port", ip: "example.com:0", wantErr: true},
		{name: "not valid domain with port", ip: "example*com:5", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Address(tt.ip); (err != nil) != tt.wantErr {
				t.Errorf("IP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseUserClusterPool(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		want1   string
		want2   string
		wantErr bool
	}{
		{name: "match", arg: "user@cluster:pool", want: "user", want1: "cluster", want2: "pool"},
		{name: "cluster match", arg: "cluster:pool", want1: "cluster", want2: "pool"},
		{name: "not match double@", arg: "user@@cluster:pool", wantErr: true},
		{name: "not match double:", arg: "user@clust:er:pool", wantErr: true},
		{name: "not match user", arg: "use-r@cluster:pool", wantErr: true},
		{name: "not match cluster", arg: "test@tes-t:pool", wantErr: true},
		{name: "not match pool", arg: "test@cluster:tes+t", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2, err := ParseUserClusterPool(tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUserPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseUserClusterPool() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ParseUserClusterPool() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("ParseUserClusterPool() got2 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestParseUserPool(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		want1   string
		wantErr bool
	}{
		{name: "match", arg: "test@test", want: "test", want1: "test"},
		{name: "not match double@", arg: "test@@test", wantErr: true},
		{name: "not match user", arg: "test-@test", wantErr: true},
		{name: "not match pool", arg: "test@tes+t", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParseUserPool(tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUserPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseUserPool() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ParseUserPool() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestParsePoolObj(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    string
		want1   string
		wantErr bool
	}{
		{
			name:  "match",
			s:     "test/some/path/with pretty permissive chars/(;)[@#$%^&].to",
			want:  "test",
			want1: "some/path/with pretty permissive chars/(;)[@#$%^&].to",
		},
		{
			name:  "match short",
			s:     "test/@%",
			want:  "test",
			want1: "@%",
		},
		{
			name:  "match long",
			s:     "test/a" + strings.Repeat("b", 1022) + "c",
			want:  "test",
			want1: "a" + strings.Repeat("b", 1022) + "c",
		},
		{
			name:  "match glob",
			s:     "test/a...",
			want:  "test",
			want1: "a...",
		},
		{
			name:    "not match no /",
			s:       "testa",
			wantErr: true,
		},
		{
			name:    "not match pool",
			s:       "1test/ab",
			wantErr: true,
		},
		{
			name:    "not match illegal char \\",
			s:       "test/a\\b",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParsePoolObj(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePoolObj() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParsePoolObj() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ParsePoolObj() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestParsePoolDot(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    string
		wantErr bool
	}{
		{name: "match", s: "Pool1/...", want: "Pool1"},
		{name: "not match double/", s: "Pool1//...", wantErr: true},
		{name: "not match pool", s: "3Pool1/...", wantErr: true},
		{name: "not match ...", s: "Pool1/a...", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePoolDot(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePoolDot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParsePoolDot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePoolObjID(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    string
		want1   string
		wantErr bool
	}{
		{name: "match", s: "1a47f8c3/4fe3f8b7", want: "1a47f8c3", want1: "4fe3f8b7"},
		{name: "not match double/", s: "1a47f8c3//4fe3f8b7", wantErr: true},
		{name: "not match pool", s: "ta47f8c3/4fe3f8b7", wantErr: true},
		{name: "not match pool", s: "1a47f8c3/qfe3f8b7", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParsePoolObjID(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePoolObjID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParsePoolObjID() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ParsePoolObjID() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestEncryptionKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	skey := base64.StdEncoding.EncodeToString(key)

	tests := []struct {
		s       any
		want    any
		name    string
		wantErr bool
	}{
		{
			name: "byte key",
			s:    key,
			want: skey,
		},
		{
			name: "string key",
			s:    skey,
			want: key,
		},
		{
			name:    "string malformed base64",
			s:       skey + "some random data",
			want:    []byte(nil),
			wantErr: true,
		},
		{
			name:    "string wrong key length",
			s:       "QSI6YXm=",
			want:    []byte(nil),
			wantErr: true,
		},
		{
			name:    "byte wrong key length",
			s:       bytes.Repeat([]byte{131, 43, 55}, 30),
			want:    "",
			wantErr: true,
		},
		{
			name:    "string not random key",
			s:       "AQIDBAECAwQBAgMEAQIDBAECAwQBAgMEAQIDBAECAwQ=",
			want:    []byte(nil),
			wantErr: true,
		},
		{
			name:    "byte not random key",
			s:       bytes.Repeat([]byte{1, 0}, 16),
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				got any
				err error
			)
			switch v := any(tt.s).(type) {
			case string:
				s, e := EncryptionKey[string, []byte](v)
				got = s
				err = e
			case []byte:
				s, e := EncryptionKey[[]byte, string](v)
				got = s
				err = e
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("EncryptionKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestParseLabelSelector(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    filter.Label
		wantErr bool
	}{
		{name: "equal", s: "k==v", want: filter.Label{Key: "k", Value: "v", Equal: true}},
		{name: "not equal", s: "key!=val", want: filter.Label{Key: "key", Value: "val", Equal: false}},
		{name: "not have =", s: "key!==val", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLabelSelector(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLabelSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLabelSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}
