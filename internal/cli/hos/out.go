// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
)

type outType string

// String is used both by fmt.Print and by Cobra in help text
func (e *outType) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *outType) Set(value string) error {
	if strings.HasPrefix(value, "fields") {
		*e = outType(value)
		return nil
	}

	switch value {
	case "default", "wide", "name", "json", "yaml":
		*e = outType(value)
		return nil
	default:
		return errors.New(`must be one of "wide", "name", "fields", "json", or "yaml"`)
	}
}

// Type is only used in help text
func (e *outType) Type() string {
	return "output"
}
