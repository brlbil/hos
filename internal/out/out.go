// SPDX-License-Identifier: MIT

// Package out provides formatted output utilities for HOS CLI commands.
// It handles table, JSON, YAML, and human-readable output formatting.
package out

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"
)

var defWriter io.Writer = os.Stdout

// Print formats and outputs data according to the specified output type.
// Supports table, JSON, YAML, and field-specific output formats.
func Print(data any, outputType string) error {
	value := reflect.ValueOf(data)
	if data == nil || (value.Kind() == reflect.Pointer && value.IsNil()) ||
		(value.Kind() == reflect.Slice && value.Len() == 0) {
		return nil
	}

	fields := []string{}
	if equalIndex := strings.Index(string(outputType), "="); equalIndex != -1 {
		caser := cases.Title(language.English, cases.NoLower)
		for _, fieldName := range strings.Split(string(outputType[equalIndex+1:]), ",") {
			fields = append(fields, caser.String(fieldName))
		}
	}

	switch outputType {
	case "default", "wide":
		fields = getFields(value, outputType)
	case "name":
		fields = []string{"Name"}
	case "json":
		return printJSON(data)
	case "yaml":
		return printYaml(data)
	}

	values := [][]string{}
	if value.Kind() != reflect.Slice {
		values = append(values, getFieldValues(value, fields...))
	} else {
		for i := 0; i < value.Len(); i++ {
			values = append(values, getFieldValues(value.Index(i), fields...))
		}
	}

	return printTable(values, fields...)
}

// getFields extracts field names from struct tags based on output key
func getFields(value reflect.Value, key string) []string {
	if value.Kind() == reflect.Slice {
		return getFields(value.Index(0), key)
	}

	fields := []string{}
	element := value
	if value.Kind() == reflect.Pointer {
		element = value.Elem()
	}

	for i := 0; i < element.NumField(); i++ {
		fieldType := element.Type().Field(i)
		keys, ok := fieldType.Tag.Lookup("print")
		// if not print tag or the key is not in the print tag, skip it
		if !ok || !strings.Contains(keys, key) {
			continue
		}
		fields = append(fields, fieldType.Name)
	}

	return fields
}

// getFieldValues extracts formatted field values from struct fields
func getFieldValues(value reflect.Value, fields ...string) []string {
	element := value
	if value.Kind() == reflect.Pointer {
		element = value.Elem()
	}

	values := []string{}
	for _, fieldName := range fields {
		field := element.FieldByName(fieldName)
		switch field.Kind() {
		case reflect.Struct:
			if timeValue, ok := field.Interface().(time.Time); ok {
				values = append(values, humanize.Time(timeValue))
			}
		case reflect.String:
			values = append(values, field.String())
		case reflect.Int:
			stringValue := strconv.FormatInt(field.Int(), 10)
			values = append(values, stringValue)
		case reflect.Int64:
			values = append(values, humanize.Bytes(uint64(field.Int())))
		case reflect.Bool:
			boolString := "no"
			if field.Bool() {
				boolString = "yes"
			}
			values = append(values, boolString)
		case reflect.Slice:
			sliceValues := []string{}
			for i := 0; i < field.Len(); i++ {
				if stringer, ok := field.Index(i).Interface().(fmt.Stringer); ok {
					sliceValues = append(sliceValues, stringer.String())
					continue
				}
				sliceValues = append(sliceValues, field.Index(i).String())
			}
			values = append(values, strings.Join(sliceValues, " "))
		}
	}
	return values
}

// printTable outputs data as a formatted table using tabwriter
func printTable(values [][]string, fields ...string) error {
	writer := &tabwriter.Writer{}
	writer.Init(defWriter, 4, 8, 4, '\t', 0)

	for i, field := range fields {
		fields[i] = strings.ToUpper(field)
	}
	fmt.Fprintln(writer, strings.Join(fields, "\t"))
	for _, rowValues := range values {
		fmt.Fprintln(writer, strings.Join(rowValues, "\t"))
	}

	return writer.Flush()
}

// printJSON outputs data as indented JSON
func printJSON(data any) error {
	encoder := json.NewEncoder(defWriter)
	encoder.SetIndent("", "    ")
	return encoder.Encode(data)
}

// printYaml outputs data as YAML
func printYaml(data any) error {
	return yaml.NewEncoder(defWriter).Encode(data)
}
