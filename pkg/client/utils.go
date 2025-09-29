// SPDX-License-Identifier: MIT

package client

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/header"
	"github.com/brlbil/hos/pkg/server"
)

// errUserNotSame
var errUserNotSame = errors.New("user not same")

func errHandlers(opts ...Options) []ErrorHandler {
	errorHandlers := []ErrorHandler{}
	for _, opt := range opts {
		eh, ok := opt.(ErrorHandler)
		if ok {
			errorHandlers = append(errorHandlers, eh)
		}
	}
	return errorHandlers
}

func handleErrors(responses []response, errorHandlers ...ErrorHandler) error {
	errs := []error{}
	var multipleErrors error

	for _, rp := range responses {
		var err error
		if rp.err != nil {
			err = convertError(rp.err)
		} else {
			err = errFromResp(rp.rsp)
		}
		if err != nil {
			errs = append(errs, err)
		}

		for _, eh := range errorHandlers {
			err = eh.HandleError(err)
		}
		if err == nil {
			continue
		}

		multipleErrors = errors.Join(multipleErrors, fmt.Errorf("%s: %w", rp.url.String(), err))
	}

	if len(errs) == len(responses) {
		var combinedError error
		for i, e := range errs {
			combinedError = errors.Join(combinedError, fmt.Errorf("%s: %w", responses[i].url.String(), e))
		}
		return combinedError
	}

	return multipleErrors
}

func errFromResp(response *http.Response) error {
	var err error
	if response.StatusCode < 400 {
		return nil
	}

	msg := []byte{}
	if response.ContentLength > 0 {
		msg, err = io.ReadAll(io.LimitReader(response.Body, response.ContentLength))
		if err != nil {
			return err
		}
	}

	switch response.StatusCode {
	case http.StatusNotFound:
		err = errT(msg, hos.ErrNotExist)
	case http.StatusConflict:
		err = errT(msg, hos.ErrExist)
	case http.StatusUnprocessableEntity:
		err = errT(msg, hos.ErrNotEmpty)
	case constant.HTTPStatusNotEqual:
		err = errT(msg, hos.ErrNotEqual)
	case http.StatusUnauthorized:
		err = errT(msg, hos.ErrNotAuthorized)
	case http.StatusForbidden:
		err = errT(msg, hos.ErrInsufficientPermissions)
	case constant.HTTPStatusNotAllowed:
		err = errT(msg, hos.ErrNotAllowed)
	case http.StatusBadRequest:
		err = errT(msg, hos.ErrBadRequest)
	case http.StatusLengthRequired:
		err = errT(msg, hos.ErrSizeRequired)
	case http.StatusUnsupportedMediaType:
		err = errT(msg, hos.ErrContentTypeRequired)
	case http.StatusRequestEntityTooLarge:
		err = errT(msg, hos.ErrContentTooLarge)
	case http.StatusExpectationFailed:
		err = errT(msg, hos.ErrDecryption)
	case constant.HTTPStatusNotInitialized:
		err = errT(msg, hos.ErrNotInitialized)
	default:
		err = &hos.HTTPError{Message: string(msg), Code: response.StatusCode}
	}

	return err
}

func errT(msg []byte, err error) error {
	if len(msg) == 0 || strings.TrimSuffix(string(msg), "\n") == err.Error() {
		return err
	}

	f := strings.Replace(string(msg), err.Error(), "%w", 1)
	return fmt.Errorf(f, err)
}

func convertError(err error) error {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return hos.ErrConnectionFailure
	}
	return err
}

func fromResponse[T any](r *http.Response) (*T, error) {
	t, err := header.Parse[T](r.Header)
	if err != nil {
		return nil, err
	}

	switch o := any(t).(type) {
	case *hos.Object:
		if r.ContentLength > 0 && o.Size != r.ContentLength {
			o.Size = r.ContentLength
		}
		o.SetBody(r.Body)
		o.SetServerAddr(r.Request.Host)

		t = any(o).(*T)
	case *hos.ServerInfo:
		ru := *r.Request.URL
		ru.Path = ""
		o.URL = &ru

		t = any(o).(*T)
	}

	return t, nil
}

func calHash(ss ...string) string {
	hash := sha256.New()
	for _, s := range ss {
		hash.Write([]byte(s))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func checkEncryptionKey(p *hos.Pool, mod ...RequestModifier) error {
	if !p.Encrypted {
		return nil
	}
	for _, md := range mod {
		if _, ekOk := md.(EncryptionKey); ekOk {
			return nil
		}
	}
	return fmt.Errorf("pool %s has encryption attribute, a encryption key must be provided, %w", p.ID, hos.ErrBadRequest)
}

func getServerConfig(responses []response) (map[string]server.Config, error) {
	var multipleErrors error
	ret := map[string]server.Config{}
	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}
		lr := io.LimitReader(r.rsp.Body, r.rsp.ContentLength)
		var conf server.Config
		if err := json.NewDecoder(lr).Decode(&conf); err != nil {
			multipleErrors = errors.Join(multipleErrors, err)
			continue
		}

		ret[r.url.Host] = conf
	}

	return ret, multipleErrors
}
