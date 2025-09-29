// SPDX-License-Identifier: MIT

// Package header handles HTTP header serialization and parsing for HOS entities.
// It converts struct fields to/from HTTP headers with custom HOS headers support.
package header

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
)

const (
	OnBehalf         = "X-Hos-On-Behalf"
	NoRedirect       = "X-Hos-No-Redirect"
	EncryptionKey    = "X-Hos-Encryption-Key"
	EncryptionNewKey = "X-Hos-Encryption-New-Key"
	NewObjectName    = "X-Hos-New-Object-Name"
	DestPool         = "X-Hos-Dest-Pool"
	DestServer       = "X-Hos-Dest-Server"
	DestToken        = "X-Hos-Dest-Token" //nolint:gosec
	OriginalHash     = "X-Hos-Original-Hash"
	SizeUnknown      = "X-Hos-Size-Unknown"
)

// Serialize converts struct fields to HTTP header key-value pairs
func Serialize(target any) map[string]string {
	result := map[string]string{}

	value := reflect.ValueOf(target)
	if target == nil || value.IsNil() {
		return result
	}

	element := value.Elem()
	for i := 0; i < element.NumField(); i++ {
		fieldType := element.Type().Field(i)
		fieldValue := element.Field(i)
		key, ok := fieldType.Tag.Lookup("header")
		if !ok && fieldValue.Kind() != reflect.Struct {
			continue
		}

		omitEmpty := false // omitempty
		if strings.HasSuffix(key, ",omitempty") {
			key = strings.TrimSuffix(key, ",omitempty")
			omitEmpty = true
		}

		switch fieldValue.Kind() {
		case reflect.Struct:
			timeValue, ok := fieldValue.Interface().(time.Time)
			if ok && (!timeValue.Equal(time.Time{}) || !omitEmpty) {
				result[key] = timeValue.Format(time.RFC1123)
			}

			maps.Copy(result, Serialize(fieldValue.Addr().Interface()))
		case reflect.String:
			stringValue := fieldValue.String()
			if stringValue != "" || !omitEmpty {
				result[key] = stringValue
			}
		case reflect.Int, reflect.Int64:
			intValue := fieldValue.Int()
			if intValue != 0 || !omitEmpty {
				result[key] = strconv.FormatInt(intValue, 10)
			}
		case reflect.Uint32, reflect.Uint64:
			uintValue := fieldValue.Uint()
			if uintValue != 0 || !omitEmpty {
				result[key] = strconv.FormatUint(uintValue, 10)
			}
		case reflect.Bool:
			boolValue := fieldValue.Bool()
			if boolValue || !omitEmpty {
				result[key] = strconv.FormatBool(boolValue)
			}
		case reflect.Slice:
			elementKind := fieldValue.Type().Elem().Kind()
			if elementKind == reflect.Struct {
				jsonBytes, err := json.Marshal(fieldValue.Addr().Interface())
				if err != nil {
					// FIXME what to do here?
					continue
				}
				encodedValue := base64.StdEncoding.EncodeToString(jsonBytes)
				if fieldValue.Len() > 0 || !omitEmpty {
					result[key] = encodedValue
				}
				continue
			}

			stringSlice := []string{}
			for i := 0; i < fieldValue.Len(); i++ {
				element := fieldValue.Index(i)
				switch elementKind {
				case reflect.String:
					stringSlice = append(stringSlice, element.String())
				case reflect.Int, reflect.Int64:
					stringSlice = append(stringSlice, strconv.FormatInt(element.Int(), 10))
				}
			}

			if len(stringSlice) > 0 || !omitEmpty {
				result[key] = strings.Join(stringSlice, ",")
			}
		case reflect.Map:
			keyValuePairs := []string{}
			for _, mapKey := range fieldValue.MapKeys() {
				keyValuePairs = append(keyValuePairs, fmt.Sprintf(`"%s"="%s"`, mapKey, fieldValue.MapIndex(mapKey)))
			}
			if len(keyValuePairs) != 0 {
				result[key] = strings.Join(keyValuePairs, "; ")
				continue
			}
			if !omitEmpty {
				result[key] = ""
			}
		}
	}

	return result
}

// Parse converts HTTP headers to struct fields based on header tags
func Parse[T any](headers http.Header) (*T, error) {
	result := new(T)
	value := reflect.ValueOf(result).Elem()
	for i := 0; i < value.NumField(); i++ {
		fieldType := value.Type().Field(i)
		key, headerExists := fieldType.Tag.Lookup("header")
		fieldValue := value.Field(i)
		if !headerExists && fieldValue.Kind() != reflect.Struct {
			continue
		}

		key = strings.TrimSuffix(key, ",omitempty")

		headerValue := headers.Get(key)
		switch fieldValue.Kind() {
		case reflect.Struct:
			if headerExists {
				if headerValue == "" {
					continue
				}

				timeValue, err := time.Parse(time.RFC1123, headerValue)
				if err != nil {
					return nil, fmt.Errorf("parsing header %s failed %s, %w", key, err.Error(), hos.ErrBadRequest)
				}
				fieldValue.Set(reflect.ValueOf(timeValue))
				continue
			}

			// if we have more than one embedded struct
			statfs, err := Parse[hos.Statfs](headers)
			if err != nil {
				return nil, err
			}
			fieldValue.Set(reflect.ValueOf(*statfs))
		case reflect.String:
			fieldValue.SetString(headerValue)
		case reflect.Int, reflect.Int64:
			if headerValue == "" {
				continue
			}
			intValue, err := strconv.ParseInt(headerValue, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing header %s failed %s, %w", key, err.Error(), hos.ErrBadRequest)
			}
			fieldValue.SetInt(intValue)
		case reflect.Uint32, reflect.Uint64:
			if headerValue == "" {
				continue
			}
			uintValue, err := strconv.ParseUint(headerValue, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing header %s failed %s, %w", key, err.Error(), hos.ErrBadRequest)
			}
			fieldValue.SetUint(uintValue)
		case reflect.Bool:
			if headerValue == "" {
				continue
			}
			boolValue, err := strconv.ParseBool(headerValue)
			if err != nil {
				return nil, fmt.Errorf("parsing header %s failed %s, %w", key, err.Error(), hos.ErrBadRequest)
			}
			fieldValue.SetBool(boolValue)
		case reflect.Slice:
			if headerValue == "" {
				continue
			}
			if fieldValue.IsNil() {
				fieldValue.Set(reflect.MakeSlice(fieldValue.Type(), 0, 0))
			}

			switch fieldValue.Type().Elem().Kind() {
			case reflect.Int:
				slice, err := headerToArray[int](headerValue)
				if err != nil {
					return nil, err
				}
				fieldValue.Set(reflect.ValueOf(slice))
			case reflect.String:
				stringArray, err := headerToArray[string](headerValue)
				if err != nil {
					return nil, err
				}
				fieldValue.Set(reflect.ValueOf(stringArray))
			case reflect.Struct:
				decodedBytes, err := base64.StdEncoding.DecodeString(headerValue)
				if err != nil {
					return nil, err
				}
				sliceInterface := fieldValue.Addr().Interface()
				if err := json.Unmarshal(decodedBytes, sliceInterface); err != nil {
					return nil, err
				}
			}

		case reflect.Map:
			if len(headerValue) == 0 {
				continue
			}

			if fieldValue.IsNil() {
				fieldValue.Set(reflect.MakeMap(fieldValue.Type()))
			}

			typeName := fieldValue.Type().Elem().Name()
			switch typeName {
			case "Permission":
				mapValue, err := headerToMap[map[string]hos.Permission](headerValue)
				if err != nil {
					return nil, err
				}
				for key, value := range mapValue {
					fieldValue.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
				}
			case "string":
				mapValue, err := headerToMap[map[string]string](headerValue)
				if err != nil {
					return nil, err
				}
				for key, value := range mapValue {
					fieldValue.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
				}
			}
		}
	}

	return result, nil
}

var validKey = regexp.MustCompile("^[A-Za-z]")

func headerToMap[M ~map[string]V, V string | hos.Permission](input string) (M, error) {
	// determine type
	var valueType V
	_, isPermission := any(valueType).(hos.Permission)
	resultMap := map[string]V{}

	for _, segment := range strings.Split(input, "; ") {
		if segment == "" {
			continue
		}

		keyValuePair := strings.SplitN(segment, "=", 2)
		if len(keyValuePair) != 2 {
			return nil, fmt.Errorf("wrong value %s, expected format key=val %w", segment, hos.ErrBadRequest)
		}

		key, value := strings.Trim(keyValuePair[0], `"`), strings.Trim(keyValuePair[1], `"`)

		// if key has ! remove it for testing
		keyForTest := strings.TrimPrefix(key, "!")
		if isPermission {
			if err := validate.PermSelector(keyForTest); err != nil {
				return nil, errors.Join(err, hos.ErrBadRequest)
			}
			if err := validate.Perm(hos.Permission(value)); err != nil {
				return nil, errors.Join(err, hos.ErrBadRequest)
			}
		} else {
			if !validKey.MatchString(keyForTest) {
				return nil, fmt.Errorf("key %s is not valid %w", key, hos.ErrBadRequest)
			}
		}

		resultMap[key] = V(value)
	}

	return resultMap, nil
}

func headerToArray[T string | int | int64](input string) ([]T, error) {
	var typeExample T
	result := []T{}
	segments := strings.Split(input, ",")

	for _, segment := range segments {
		switch any(typeExample).(type) {
		case string:
			return any(segments).([]T), nil
		case int:
			intValue, err := strconv.ParseInt(segment, 10, 32)
			if err != nil {
				return nil, err
			}
			result = append(result, any(int(intValue)).(T))
		case int64:
			int64Value, err := strconv.ParseInt(segment, 10, 64)
			if err != nil {
				return nil, err
			}
			result = append(result, any(int64Value).(T))
		}
	}

	return result, nil
}
