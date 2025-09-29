// SPDX-License-Identifier: MIT

package tests

import (
	"path"
	"testing"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/pkg/crypto"
	"github.com/brlbil/hos/pkg/id"
)

const (
	adminUser = constant.AdminUser
	anonUser  = constant.AnonUser

	User1 = "user1"
	User2 = "user2"
	User3 = "user3"

	User1ID = "8c518555"
	User2ID = "1558d4ef"
	User3ID = "625fe479"
)

var prvKeys = map[string][]string{
	adminUser: {
		"G5fFt43hzUhl7zEMVCG5NufCfGwelu1FxGLn4wvnWKc=",
		"JdgshqE0rL6ejv1V+3PcHgmH5fHHdXmlaOh6gWHffPY=",
	},
	User1: {"Ps8jtjXJx3RY+BRolW0OcjdYx1jnB61HYuKWiSeIJs4="},
	User2: {
		"pBNlMLrLEXRFyKgUm7pvNQaqHW1gxrzBmN6CJCKmKfw=",
		"MpCg7BqUJQT0CqryxcFUDSXPmxNN7MNP8z1vbvgydXU=",
		"I75dFHf65hfr5H02tCnHHptTL+nbAxf/71pBuktt0EE=",
	},
	User3:    {"qJSalzH5UY6mvXrLtaE0gEk70SHizttunPkO1/jPRoc="},
	anonUser: {"nXvGemmIatTUFScSVnoIE07uf9OlLnvo9Eiw/P2DL8g="},
}

var pubKeys = map[string][]string{
	adminUser: {
		"gVlthiBGZR8KiWNo0ofmwb5NuWI3xCGzrjW7JdL1izw=",
		"GLFWlKoXvHi3gtnY4atgR8Ix90SImQxL0y6YaIlY1Ow=",
	},
	User1: {"A824E2RoFRGTv057tBas+ICDDEuF1OkT5CUbbqcyEuY="},
	User2: {
		"o5s11YIt3F3egtVdKTqibT4G6GrCZ1ypk5z946K/Omw=",
		"7HNYVtc1IRPgxprpeYg0NA8daVyKoBxE5GpWpT0caLY=",
		"K+Ko4AxaTCS+ZCP1lkUlY6xn8ZzvD5l+MvM8a/6YVxI=",
	},
	User3:    {"ScjT0Csdt18nZXxjtl9hltobRaPVkVsKmYYDSkYqoe4="},
	anonUser: {"z/QFeMPpxrZqBs/p0tWWM5r4Vh3Cz9H0JxuFqPmkt58="},
}

func pk(u string, inx ...int) []crypto.PublicKey {
	tt := []crypto.PublicKey{}
	if len(inx) == 0 {
		inx = []int{0}
	}
	for _, i := range inx {
		del := i < 0
		if del {
			i *= -1
		}
		pk, err := crypto.ParsePublicKey(pubKeys[u][i])
		if err != nil {
			panic(err)
		}

		if del {
			b := make([]byte, 33)
			b[0] = 33
			copy(b[1:], pk)
			pk = crypto.PublicKey(b)
		}

		tt = append(tt, pk)
	}
	return tt
}

func PrivateKey(user string) string {
	pks, ok := prvKeys[user]
	if !ok {
		return ""
	}
	return pks[0]
}

func ParsePrivateKey(user string, t *testing.T) crypto.PrivateKey {
	t.Helper()

	pks, ok := prvKeys[user]
	if !ok {
		t.Fatalf("user %s is not defined, does not have a private key", user)
	}

	pk, err := crypto.ParsePrivateKey(pks[0])
	if err != nil {
		t.Fatalf("private key parsing failed: %s", err)
	}

	return pk
}

func User(u string, inx ...int) *hos.User {
	if len(inx) == 0 {
		inx = []int{0}
	}

	if _, ok := pubKeys[u]; !ok {
		return &hos.User{Name: u, ID: id.Gen(u)}
	}

	return &hos.User{Name: u, ID: id.Gen(u), PublicKeys: pk(u, inx...)}
}

func PID(uid, pool string) string {
	return id.Gen(uid, pool)
}

func OID(uid, pool, obj string) string {
	return id.Gen(PID(uid, pool), obj)
}

func OPath(uid, pool, obj string) string {
	return path.Join(PID(uid, pool), OID(uid, pool, obj))
}

func OLinkPath(uid, pool, oid string) string {
	return path.Join(PID(uid, pool), oid)
}
