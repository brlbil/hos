// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/internal/iofactory"
	"github.com/brlbil/hos/internal/validate"
	"github.com/muesli/termenv"
)

// Options is the generic interface for client configuration and request modifiers
type Options interface {
	Option()
}

// ErrorHandler interface allows modifying or suppressing errors from client operations
type ErrorHandler interface {
	HandleError(error) error
}

type ignoreErrorsExcept []error

func (ignoreErrorsExcept) Option() {}

func (errs ignoreErrorsExcept) HandleError(err error) error {
	for _, e := range errs {
		if errors.Is(err, e) {
			return err
		}
	}
	return nil
}

// IgnoreErrorsExcept ignores all errors except the specified ones
func IgnoreErrorsExcept(errs ...error) ignoreErrorsExcept {
	return ignoreErrorsExcept(errs)
}

type ignoreErrors []error

func (ignoreErrors) Option() {}

func (errs ignoreErrors) HandleError(err error) error {
	for _, e := range errs {
		if errors.Is(err, e) {
			return nil
		}
	}
	return err
}

// IgnoreErrors ignores the specified errors and continues operation
func IgnoreErrors(errs ...error) ignoreErrors {
	return ignoreErrors(errs)
}

type warnErrors []error

func (warnErrors) Option() {}

func (errs warnErrors) HandleError(err error) error {
	out := termenv.NewOutput(os.Stdout)
	for _, e := range errs {
		if errors.Is(err, e) {
			fmt.Printf("%s %s\n", out.String("Warning:").Foreground(out.Color("11")), err)
			return nil
		}
	}
	return err
}

// WarnErrors prints warnings for the specified errors but continues operation
func WarnErrors(errs ...error) warnErrors {
	return warnErrors(errs)
}

// RequestModifier allows modifying HTTP requests before they are sent
type RequestModifier interface {
	ModifyRequest(*http.Request) error
}

func modifiers(opts ...Options) []RequestModifier {
	mods := []RequestModifier{}
	for _, opt := range opts {
		m, ok := opt.(RequestModifier)
		if ok {
			mods = append(mods, m)
		}
	}
	return mods
}

// NoRedirect prevents following redirects for linked pools
func NoRedirect() Headers {
	return Headers(map[string]string{header.NoRedirect: "_"})
}

// EncryptionKey provides the key for encrypted pool operations
type EncryptionKey []byte

var (
	_ RequestModifier = EncryptionKey(nil)
	_ Options         = EncryptionKey(nil)
)

func (EncryptionKey) Option() {}

func (key EncryptionKey) ModifyRequest(r *http.Request) error {
	se, err := validate.EncryptionKey[[]byte, string](key)
	if err != nil {
		return err
	}
	r.Header.Set(header.EncryptionKey, se)
	return nil
}

type newEncryptionKey []byte

var (
	_ RequestModifier = newEncryptionKey(nil)
	_ Options         = newEncryptionKey(nil)
)

func (newEncryptionKey) Option() {}

func (key newEncryptionKey) ModifyRequest(r *http.Request) error {
	se, err := validate.EncryptionKey[[]byte, string](key)
	if err != nil {
		return err
	}
	r.Header.Set(header.EncryptionNewKey, se)
	return nil
}

// OnBehalf allows admin users to perform operations on behalf of another user
type OnBehalf string

var (
	_ RequestModifier = OnBehalf("")
	_ Options         = OnBehalf("")
)

func (OnBehalf) Option() {}

func (user OnBehalf) ModifyRequest(r *http.Request) error {
	r.Header.Set(header.OnBehalf, string(user))
	return nil
}

// Headers allows setting custom HTTP headers for requests
type Headers map[string]string

var (
	_ RequestModifier = Headers{}
	_ Options         = Headers{}
)

func (Headers) Option() {}

func (h Headers) ModifyRequest(r *http.Request) error {
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return nil
}

type readClosers struct {
	rc iofactory.ReadClosers
}

var (
	_ RequestModifier = &readClosers{}
	_ Options         = &readClosers{}
)

func (readClosers) Option() {}

func (rcs readClosers) ModifyRequest(r *http.Request) error {
	server := r.Host
	rc, err := rcs.rc.New(server)
	if err != nil {
		return err
	}
	r.Body = rc
	r.GetBody = func() (io.ReadCloser, error) {
		return rcs.rc.New(server)
	}
	return nil
}

type jsonBody []byte

var (
	_ RequestModifier = jsonBody{}
	_ Options         = jsonBody{}
)

func (jsonBody) Option() {}

func (jb jsonBody) ModifyRequest(r *http.Request) error {
	r.Body = io.NopCloser(bytes.NewReader(jb))
	r.Header.Set("Content-Type", "application/json")
	return nil
}

// byServerJSONBody sets request body with the json encoded data
// that matching the server URL.Host
type byServerJSONBody map[string]any

var (
	_ RequestModifier = byServerJSONBody{}
	_ Options         = byServerJSONBody{}
)

func (byServerJSONBody) Option() {}

func (jb byServerJSONBody) ModifyRequest(r *http.Request) error {
	a, ok := jb[r.URL.Host]
	if !ok {
		return fmt.Errorf("host %s data is not exists", r.URL.Host)
	}
	b, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("host %s, encoding failed: %w", r.URL.Host, err)
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return nil
}

type urlQueries map[string]string

var (
	_ RequestModifier = urlQueries{}
	_ Options         = urlQueries{}
)

func (urlQueries) Option() {}

func (uq urlQueries) ModifyRequest(r *http.Request) error {
	val := r.URL.Query()
	for k, v := range uq {
		if k == "" || v == "" {
			continue
		}
		val.Set(k, v)
	}
	r.URL.RawQuery = val.Encode()
	return nil
}
