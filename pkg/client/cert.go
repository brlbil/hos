// SPDX-License-Identifier: MIT

package client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func GetCertificate(addr string) (string, error) {
	clt := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint: gosec
		},
	}
	rsp, err := clt.Get("https://" + addr + "/ca")
	if err != nil {
		return "", fmt.Errorf("%s %w", addr, err)
	}

	if rsp.StatusCode != 200 {
		return "", fmt.Errorf("%s expected 200, got %d", addr, rsp.StatusCode)
	}

	r := io.LimitReader(rsp.Body, rsp.ContentLength)
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("%s %w", addr, err)
	}

	s := strings.Trim(string(data), "\n")

	return s, nil
}
