// SPDX-License-Identifier: MIT

// Package utils provides utility functions for HOS internal operations.
// It includes struct comparison, logging helpers, and common internal utilities.
package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Diff compares two structs and returns differences for User, Object, and Pool types
func Diff(a, b any) error {
	// check if they are the same type
	typeA := reflect.TypeOf(a)
	typeB := reflect.TypeOf(b)
	if typeA != typeB {
		return fmt.Errorf("type %s, is not same type as %s", typeA, typeB)
	}

	valueA := reflect.ValueOf(a)
	valueB := reflect.ValueOf(b)
	if (a == nil && b == nil) || (valueA.IsNil() && valueB.IsNil()) {
		return nil
	}

	differences := []string{}

	elementA := valueA.Elem()
	elementB := valueB.Elem()
	for i := 0; i < elementA.NumField(); i++ {
		fieldA := elementA.Field(i)
		fieldB := elementB.Field(i)

		fieldTypeA := elementA.Type().Field(i)
		key, ok := fieldTypeA.Tag.Lookup("diff")
		if ok && key == "ignore" {
			continue
		}

		switch fieldA.Kind() {
		case reflect.Struct:
			timeA, ok1 := fieldA.Interface().(time.Time)
			timeB, ok2 := fieldB.Interface().(time.Time)
			if ok1 && ok2 && !timeA.Equal(timeB) {
				differences = append(differences,
					fmt.Sprintf("  %s: %s != %s", fieldTypeA.Name, timeA.Format(time.RFC3339Nano), timeB.Format(time.RFC3339Nano)))
			}

		case reflect.String:
			stringA := fieldA.String()
			stringB := fieldB.String()
			if stringA != stringB {
				differences = append(differences, fmt.Sprintf("  %s: %s != %s", fieldTypeA.Name, stringA, stringB))
			}
		case reflect.Int, reflect.Int64:
			intA := fieldA.Int()
			intB := fieldB.Int()
			if intA != intB {
				differences = append(differences, fmt.Sprintf("  %s: %d != %d", fieldTypeA.Name, intA, intB))
			}
		case reflect.Slice:
			sliceA := []string{}
			sliceB := []string{}

			for i := 0; i < fieldA.Len(); i++ {
				sliceA = append(sliceA, fmt.Sprintf("%x", fieldA.Index(i).Bytes()))
			}
			for i := 0; i < fieldB.Len(); i++ {
				sliceB = append(sliceB, fmt.Sprintf("%x", fieldB.Index(i).Bytes()))
			}

			slices.Sort(sliceA)
			slices.Sort(sliceB)

			lengthA := len(sliceA)
			lengthB := len(sliceB)

			minIndex := lengthA
			if lengthB < lengthA {
				minIndex = lengthB
			}

			sliceDiffs := []string{}

			for i, valueA := range sliceA[:minIndex] {
				valueB := sliceB[i]
				if valueA != valueB {
					sliceDiffs = append(sliceDiffs, fmt.Sprintf("    = %d: %s != %s", i, valueA, valueB))
				}
			}

			if minIndex < lengthA {
				for i, valueA := range sliceA[minIndex:] {
					sliceDiffs = append(sliceDiffs, fmt.Sprintf("    + %d: %s", i+minIndex, valueA))
				}
			}

			if minIndex < lengthB {
				for i, valueB := range sliceB[minIndex:] {
					sliceDiffs = append(sliceDiffs, fmt.Sprintf("    - %d: %s", i+minIndex, valueB))
				}
			}

			if len(sliceDiffs) > 0 {
				differences = append(differences, fmt.Sprintf("  %s:", fieldTypeA.Name))
				differences = append(differences, sliceDiffs...)
			}

		case reflect.Map:
			mapDiffs := []string{}

			mapKeysA := []string{}
			mapKeysB := []string{}

			mapA := map[string]string{}
			mapB := map[string]string{}

			for _, keyA := range fieldA.MapKeys() {
				mapKeysA = append(mapKeysA, keyA.String())
				mapA[keyA.String()] = fieldA.MapIndex(keyA).String()
			}

			for _, keyB := range fieldB.MapKeys() {
				mapKeysB = append(mapKeysB, keyB.String())
				mapB[keyB.String()] = fieldB.MapIndex(keyB).String()
			}

			slices.Sort(mapKeysA)
			slices.Sort(mapKeysB)

			for _, key := range mapKeysA {
				valueA := mapA[key]
				valueB, ok := mapB[key]
				if !ok {
					mapDiffs = append(mapDiffs, fmt.Sprintf("    + %s: %s", key, valueA))
					continue
				}
				if valueA != valueB {
					mapDiffs = append(mapDiffs, fmt.Sprintf("    = %s: %s != %s", key, valueA, valueB))
				}
			}

			for _, key := range mapKeysB {
				valueB := mapB[key]
				_, ok := mapA[key]
				if !ok {
					mapDiffs = append(mapDiffs, fmt.Sprintf("    - %s: %s", key, valueB))
				}
			}

			if len(mapDiffs) > 0 {
				differences = append(differences, fmt.Sprintf("  %s:", fieldTypeA.Name))
				differences = append(differences, mapDiffs...)
			}
		}
	}

	var err error
	for i, diffLine := range differences {
		if i == 0 {
			typeParts := strings.Split(typeA.String(), ".")
			err = fmt.Errorf("%s not equal", typeParts[len(typeParts)-1])
		}
		err = errors.Join(err, errors.New(diffLine))
	}

	return err
}

// DiscardLogger creates a logger that discards all output
func DiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// CountWriter tracks the number of bytes written
type CountWriter int64

// Write counts bytes written and implements io.Writer
func (cw *CountWriter) Write(buffer []byte) (int, error) {
	bufferLength := len(buffer)
	*cw += CountWriter(bufferLength)
	return bufferLength, nil
}
