// SPDX-License-Identifier: MIT

// Package validate provides input validation functions for HOS entities.
// It validates names, IDs, paths, labels, permissions, and other user inputs.
package validate

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/filter"
)

const (
	poolNameMatch   = `[A-Za-z][A-Za-z0-9_-]{0,23}[A-Za-z0-9]`
	keyMatch        = `[A-Za-z][A-Za-z0-9\/\-_\s]{0,23}[A-Za-z0-9]`
	labelValueMatch = `[A-Za-z][A-Za-z0-9\/\-_\s,\.;:]{0,253}[A-Za-z0-9]`
	attrValueMatch  = `[[:print:]]{1,1000}`
	userNameMatch   = `[A-Za-z][A-Za-z0-9]{1,9}`
	pathMatch       = `[^\x00-\x1F\/\\:*?"<>|][^\x00-\x1F\\:*?"<>|]{0,512}[^\x00-\x1F\\:*?"<>|]{0,510}[^.\x00-\x1F\/\\:*?"<>| ]`
)

var (
	expOneLetter       = regexp.MustCompile(`^[a-zA-Z]$`)
	expPool            = regexp.MustCompile(fmt.Sprintf(`^%s$`, poolNameMatch))
	expObj             = regexp.MustCompile(fmt.Sprintf(`^%s$`, pathMatch))
	expID              = regexp.MustCompile(`^[[:xdigit:]]{8}$`)
	expPoolDot         = regexp.MustCompile(fmt.Sprintf(`^(%s)/(\.{3})$`, poolNameMatch))
	expPoolObj         = regexp.MustCompile(fmt.Sprintf(`^(%s)/(%s?)$`, poolNameMatch, pathMatch))
	expPoolObjID       = regexp.MustCompile("^([[:xdigit:]]{8})/([[:xdigit:]]{8})$")
	expClusterPool     = regexp.MustCompile(fmt.Sprintf("^(%s):(%s)$", userNameMatch, poolNameMatch))
	expUser            = regexp.MustCompile(fmt.Sprintf("^%s$", userNameMatch))
	expUserPool        = regexp.MustCompile(fmt.Sprintf(`^(%s)@(%s)$`, userNameMatch, poolNameMatch))
	expUserClusterPool = regexp.MustCompile(fmt.Sprintf("^(%s)@(%s):(%s)$", userNameMatch, userNameMatch, poolNameMatch))
	expKey             = regexp.MustCompile(fmt.Sprintf("^%s$", keyMatch))
	expLabel           = regexp.MustCompile(fmt.Sprintf("^([a-zA-Z]|%s)=([a-zA-Z]|%s)$", keyMatch, labelValueMatch))
	expLabelSel        = regexp.MustCompile(fmt.Sprintf("^([a-zA-Z]|%s)(!=|==)([a-zA-Z]|%s)$", keyMatch, labelValueMatch))
	expLabelVal        = regexp.MustCompile(fmt.Sprintf("^([a-zA-Z]|%s)$", labelValueMatch))
	expAttr            = regexp.MustCompile(fmt.Sprintf("^(%s)=(%s)$", keyMatch, attrValueMatch))
	expPerm            = regexp.MustCompile(fmt.Sprintf("^(\\*|%s):(r|w)$", userNameMatch))
	expDomain          = regexp.MustCompile(`^([a-zA-Z0-9]+(-[a-zA-Z0-9]+)*\.)+[a-zA-Z]{2,}$`)
)

// Pool validates pool names against naming rules
func Pool(name string) error {
	if expPool.MatchString(name) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid pool name, must, start with a letter, only contain letters numbers '-' '_',
ends with a letter or number, not be shorter then 2 and longer then 25 characters`, name)
}

// Object validates object names against naming rules
func Object(name string) error {
	if expObj.MatchString(name) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid object name, must, not start with illegal chars ascii(0-31),/,\,:,*,?,",<,>,|,
not contain illegal chars except / in the middle, not be ends with illegal chars plus space and .
not be shorter then 2 and longer then 1024 characters`, name)
}

// ID validates 8-character hexadecimal IDs
func ID(id string) error {
	if expID.MatchString(id) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid id, must, only contain hexadecimal digits,
not be longer then 8 characters`, id)
}

// key validates generic key strings
func key(key, str string) error {
	if expKey.MatchString(str) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid %s key, must, start with a letter, only contain letters numbers '/' '-' '_' ' '
ends with a letter or number, not be shorter then 2 and longer then 25 characters`, str, key)
}

// Label validates label keys
func Label(label string) error {
	if expOneLetter.MatchString(label) {
		return nil
	}
	return key("label", label)
}

// LabelValue validates label values
func LabelValue(val string) error {
	if expLabelVal.MatchString(val) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid label value, must, start with a letter,
only contain letters numbers '/' '-' '_' ' ' ',' '.' ';' ':',
ends with a letter or number, not be shorter then 2 and longer then 255 characters`, val)
}

// ParseLabel extracts key and value from label string
func ParseLabel(label string) (string, string, error) {
	submatches := expLabel.FindStringSubmatch(label)
	if len(submatches) == 3 {
		return submatches[1], submatches[2], nil
	}
	parts := strings.Split(label, "=")
	if partsLength := len(parts); partsLength != 2 {
		return "", "", fmt.Errorf("%s is not a valid label key=value, contains %d =", label, (partsLength - 1))
	}

	if err := Label(parts[0]); err != nil {
		return "", "", err
	}

	return "", "", fmt.Errorf(`%s is not a valid label value, must, start with a letter,
only contain letters numbers '/' '-' '_' ' ' ',' '.' ';' ':',
ends with a letter or number, not be shorter then 2 and longer then 255 characters`, parts[1])
}

// ParseAttr extracts key and value from attribute string
func ParseAttr(attr string) (string, string, error) {
	submatches := expAttr.FindStringSubmatch(attr)
	if len(submatches) == 3 {
		return submatches[1], submatches[2], nil
	}
	parts := strings.SplitN(attr, "=", 2)
	if len(parts) == 1 {
		return "", "", fmt.Errorf("%s is not a valid attribute key=value, contains no =", attr)
	}

	if err := key("attribute", parts[0]); err != nil {
		return "", "", err
	}

	return "", "", fmt.Errorf(`%s is not a valid attribute value,
must, contain any visible character, not be longer then 1000 characters`, parts[1])
}

// User validates user names
func User(userName string) error {
	if expUser.MatchString(userName) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid user name, must, start with a letter, only contain letters or numbers,
not be shorter then 2 and longer then 10 characters`, userName)
}

// Cluster validates cluster names
func Cluster(clusterName string) error {
	if expUser.MatchString(clusterName) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid cluster name, must, start with a letter, only contain letters or numbers,
not be shorter then 2 and longer then 10 characters`, clusterName)
}

// PermSelector validates permission selectors (* or user names)
func PermSelector(selector string) error {
	if selector == "*" {
		return nil
	}
	if expUser.MatchString(selector) {
		return nil
	}
	return fmt.Errorf(`%s is not a valid permission selector, must be either * or a user name,
user name must start with a letter, only contain letters or numbers, not be longer than 10 characters`, selector)
}

const (
	read  hos.Permission = "r"
	write hos.Permission = "w"
)

// Perm validates permission values (r or w)
func Perm(perm hos.Permission) error {
	switch perm {
	case read, write:
		return nil
	default:
		return fmt.Errorf("%s is not a valid permission, must be one of 'r' or 'w'", perm)
	}
}

// ParsePerm extracts selector and permission from permission string
func ParsePerm(perm string) (string, hos.Permission, error) {
	submatches := expPerm.FindStringSubmatch(perm)
	if len(submatches) == 3 {
		return submatches[1], hos.Permission(submatches[2]), nil
	}
	parts := strings.SplitN(perm, ":", 2)
	if len(parts) == 1 {
		return "", "", fmt.Errorf("%s is not a valid permission selector:value, contains no ':'", perm)
	}

	if err := PermSelector(parts[0]); err != nil {
		return "", "", err
	}

	return "", "", Perm(hos.Permission(parts[1]))
}

// Address validates network addresses (IP:port or domain:port)
func Address(addr string) error {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) == 2 {
		if _, err := netip.ParseAddrPort(addr); err == nil {
			return nil
		}
		if port, err := strconv.Atoi(parts[1]); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("%s is not a valid port", parts[1])
		}
		if !expDomain.MatchString(parts[0]) {
			return fmt.Errorf("%s is not a valid ip address or domain name", parts[0])
		}
		return nil
	}
	if _, err := netip.ParseAddr(addr); err == nil {
		return nil
	}
	if !expDomain.MatchString(addr) {
		return fmt.Errorf("%s is not a valid ip address or domain name", addr)
	}

	return nil
}

// ParseUserPool extracts user and pool from user@pool format
func ParseUserPool(name string) (string, string, error) {
	submatches := expUserPool.FindStringSubmatch(name)
	if len(submatches) == 3 {
		return submatches[1], submatches[2], nil
	}
	parts := strings.Split(name, "@")
	if partsLength := len(parts); partsLength != 2 {
		return "", "", fmt.Errorf("%s is not a valid user@pool, contains %d @", name, (partsLength - 1))
	}

	if err := User(parts[0]); err != nil {
		return "", "", err
	}

	return "", "", Pool(parts[1])
}

// ParseUserClusterPool extracts user, cluster, and pool from user@cluster:pool format
func ParseUserClusterPool(name string) (string, string, string, error) {
	submatches := expUserClusterPool.FindStringSubmatch(name)
	if len(submatches) == 4 {
		return submatches[1], submatches[2], submatches[3], nil
	}

	// should be cluser:pool
	if !strings.Contains(name, "@") {
		submatches = expClusterPool.FindStringSubmatch(name)
		if len(submatches) == 3 {
			return "", submatches[1], submatches[2], nil
		}

		parts := strings.Split(name, ":")
		if partsLength := len(parts); partsLength != 2 {
			return "", "", "", fmt.Errorf("%s is not a valid cluser:pool, contains %d ':'", name, (partsLength - 1))
		}

		if err := Cluster(parts[0]); err != nil {
			return "", "", "", err
		}

		return "", "", "", Pool(parts[1])
	}

	parts := strings.Split(name, "@")
	if partsLength := len(parts); partsLength != 2 {
		return "", "", "", fmt.Errorf("%s is not a valid user@pool, contains %d @", name, (partsLength - 1))
	}

	if err := User(parts[0]); err != nil {
		return "", "", "", err
	}

	clusterParts := strings.Split(parts[1], ":")
	if clusterPartsLength := len(clusterParts); clusterPartsLength != 2 {
		return "", "", "", fmt.Errorf("%s is not a valid user@cluster:pool, contains %d ':'", name, (clusterPartsLength - 1))
	}

	if err := Cluster(clusterParts[0]); err != nil {
		return "", "", "", err
	}

	return "", "", "", Pool(clusterParts[1])
}

// ParsePoolObj extracts pool and object from pool/object format
func ParsePoolObj(name string) (string, string, error) {
	submatches := expPoolObj.FindStringSubmatch(name)
	if len(submatches) == 3 {
		return submatches[1], submatches[2], nil
	}
	parts := strings.SplitN(name, "/", 2)
	if partsLength := len(parts); partsLength != 2 {
		return "", "", fmt.Errorf("%s is not a valid pool/object, contains no /", name)
	}

	if err := Pool(parts[0]); err != nil {
		return "", "", err
	}

	return "", "", fmt.Errorf(`%s is not a valid object name selector, must, not start with illegal chars ascii(0-31),/,\,:,*,?,",<,>,|,
not contain illegal chars except / , not be longer then 1024 characters`, name)
}

// ParsePoolDot extracts pool from pool/... format
func ParsePoolDot(pool string) (string, error) {
	submatches := expPoolDot.FindStringSubmatch(pool)
	if len(submatches) == 3 {
		return submatches[1], nil
	}
	parts := strings.Split(pool, "/")
	if partsLength := len(parts); partsLength != 2 {
		return "", fmt.Errorf("%s is not a valid pool/..., contains %d /", pool, (partsLength - 1))
	}

	if err := Pool(parts[0]); err != nil {
		return "", err
	}

	return "", fmt.Errorf("%s is not equal to '...'", parts[1])
}

// ParsePoolObjID extracts pool and object IDs from poolID/objectID format
func ParsePoolObjID(name string) (string, string, error) {
	submatches := expPoolObjID.FindStringSubmatch(name)
	if len(submatches) == 3 {
		return submatches[1], submatches[2], nil
	}
	parts := strings.Split(name, "/")
	if partsLength := len(parts); partsLength != 2 {
		return "", "", fmt.Errorf("%s is not a valid pool/object id, contains %d /", name, (partsLength - 1))
	}

	if err := ID(parts[0]); err != nil {
		return "", "", err
	}

	return "", "", ID(parts[1])
}

// EncryptionKey validates and converts encryption keys with randomness checks
func EncryptionKey[T string | []byte, R []byte | string](t T) (R, error) {
	var result R
	switch value := any(t).(type) {
	case string:
		bytes, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return result, err
		}
		if len(bytes) != 32 {
			return result, fmt.Errorf("encryption key length is wrong %w", hos.ErrBadRequest)
		}
		// reject if key is not random, this is not a definite proof of randomness
		// but it is good to have it than not having it
		if err := checkKeyRandomness(bytes); err != nil {
			return result, err
		}
		result, _ = any(bytes).(R)
	case []byte:
		if len(value) != 32 {
			return result, fmt.Errorf("encryption key length is wrong %w", hos.ErrBadRequest)
		}
		// reject if key is not random, this is not a definite proof of randomness
		// but it is good to have it than not having it
		if err := checkKeyRandomness(value); err != nil {
			return result, err
		}
		encodedString := base64.StdEncoding.EncodeToString(value)
		result, _ = any(encodedString).(R)
	}
	return result, nil
}

// checkKeyRandomness performs basic randomness checks on encryption keys
func checkKeyRandomness(key []byte) error {
	// not all equal
	allEqual := true
	for i := 1; i < len(key); i++ {
		if key[i] != key[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return fmt.Errorf("key is not random, all bytes equal")
	}

	// byte frequency chi-square against uniform (weak heuristic)
	var frequency [256]int
	for _, bytVal := range key {
		frequency[bytVal]++
	}
	var chiSquare float64
	expected := float64(len(key)) / 256.0
	for _, count := range frequency {
		difference := float64(count) - expected
		chiSquare += difference * difference / expected
	}
	if chiSquare < 100 || chiSquare > 600 {
		return fmt.Errorf("key is not random enough")
	}

	return nil
}

// ParseLabelSelector extracts label filter from key==value or key!=value format
func ParseLabelSelector(selector string) (filter.Label, error) {
	submatches := expLabelSel.FindStringSubmatch(selector)
	if len(submatches) == 4 {
		return filter.Label{Key: submatches[1], Value: submatches[3], Equal: submatches[2] == "=="}, nil
	}
	equalParts := strings.Split(selector, "==")
	notEqualParts := strings.Split(selector, "!=")
	equalLength := len(equalParts)
	notEqualLength := len(notEqualParts)
	if equalLength != 2 && notEqualLength != 2 {
		return filter.Label{},
			fmt.Errorf("%s is not a valid label key=value, contains %d != and %d ==", selector, (equalLength - 1), (notEqualLength - 1))
	}

	if notEqualLength == 2 {
		equalParts = notEqualParts
	}

	if err := Label(equalParts[0]); err != nil {
		return filter.Label{}, err
	}

	return filter.Label{}, fmt.Errorf(`%s is not a valid label value, must, start with a letter,
only contain letters numbers '/' '-' '_' ' ' ',' '.' ';' ':',
ends with a letter or number, not be shorter then 2 and longer then 255 characters`, equalParts[1])
}
