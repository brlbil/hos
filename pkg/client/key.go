// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
	"github.com/brlbil/hos/internal/enc"
)

// CreateKey creates an encryption key on the servers
// in order to create key from an existing key EncryptionKey option must also be provided
func (c *Client) CreateKey(ctx context.Context, key []byte, opts ...Options) error {
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return err
	}

	responses := c.doP(ctx, "PUT", constant.KeyAPIPrefix, c.servers, append(modifiers(opts...), newEncryptionKey(key))...)
	return handleErrors(responses, append(errHandlers(opts...), IgnoreErrors(hos.ErrExist))...)
}

type keyInfo struct {
	key     hos.Key
	servers map[string]struct{}
}

// ListKeys list encryption keys
func (c *Client) ListKeys(ctx context.Context, opts ...Options) ([]hos.Key, error) {
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return nil, err
	}

	responses := c.doP(ctx, "GET", constant.KeyAPIPrefix, c.servers, modifiers(opts...)...)
	if err := handleErrors(responses, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	allKeys, err := getKeys(responses)
	if err != nil {
		return nil, err
	}

	keyInfoMap := map[string]*keyInfo{}
	for server, keys := range allKeys {
		for _, key := range keys {
			info, ok := keyInfoMap[key.Signature]
			if !ok {
				info = &keyInfo{key: key, servers: make(map[string]struct{}, 5)}
				keyInfoMap[key.Signature] = info
			}
			info.servers[server] = struct{}{}
		}
	}

	results := []hos.Key{}
	var multipleErrors error

	for signature, info := range keyInfoMap {
		for server := range allKeys {
			// check if key is exists on all servers
			if _, ok := info.servers[server]; !ok {
				err := fmt.Errorf("%s key %s... %w", server, signature[:8], hos.ErrNotExist)
				// give err to error handlers, here caller might be supresing the hos.ErrNotExist error
				for _, eh := range errHandlers(opts...) {
					err = eh.HandleError(err)
				}
				// if err not supresed than add it
				if err != nil {
					multipleErrors = errors.Join(multipleErrors, err)
				}
			}
		}
		results = append(results, info.key)
	}

	if multipleErrors != nil {
		return nil, multipleErrors
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})

	return results, nil
}

func getKeys(responses []response) (map[string][]hos.Key, error) {
	var multipleErrors error
	ret := map[string][]hos.Key{}
	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}
		lr := io.LimitReader(r.rsp.Body, r.rsp.ContentLength)
		var keys []hos.Key
		if err := json.NewDecoder(lr).Decode(&keys); err != nil {
			multipleErrors = errors.Join(multipleErrors, err)
			continue
		}

		ret[r.url.String()] = keys
	}

	if multipleErrors != nil {
		return nil, multipleErrors
	}

	return ret, nil
}

// RestoreKey creates an encryption key from a backup data
func (c *Client) RestoreKey(ctx context.Context, data map[string]enc.Key, opts ...Options) error {
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return err
	}

	// filter out servers that do not have key data
	servers := []url.URL{}
	jsonData := map[string]any{}
	prevKid := ""
	for _, srv := range c.servers {
		key, ok := data[srv.Host]
		if !ok || len(key.Data) == 0 {
			fmt.Println(srv.Host, "skipped")
			continue
		}
		if prevKid != "" && prevKid != key.ID {
			return fmt.Errorf("key id %s does not match the previous key id %s", key.ID, prevKid)
		}

		prevKid = key.ID
		servers = append(servers, srv)
		jsonData[srv.Host] = any(key)
	}

	modifiersList := append(modifiers(opts...), byServerJSONBody(jsonData))
	responses := c.doP(ctx, "PUT", constant.KeyAPIDataPrefix, servers, modifiersList...)
	return handleErrors(responses, append(errHandlers(opts...), IgnoreErrors(hos.ErrExist))...)
}

// GetServerKeys returns server enc key datas
func (c *Client) GetServerKeys(ctx context.Context, opts ...Options) (map[string][]enc.Key, error) {
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return nil, err
	}

	// get keys
	responses := c.doP(ctx, "GET", constant.KeyAPIDataPrefix, c.servers, modifiers(opts...)...)
	if err := handleErrors(responses); err != nil {
		return nil, err
	}

	var multipleErrors error
	ret := map[string][]enc.Key{}
	for _, r := range responses {
		if r.err != nil || (r.rsp != nil && r.rsp.StatusCode >= 400) {
			continue
		}

		lr := io.LimitReader(r.rsp.Body, r.rsp.ContentLength)
		var keys []enc.Key
		if err := json.NewDecoder(lr).Decode(&keys); err != nil {
			multipleErrors = errors.Join(multipleErrors, err)
			continue
		}

		ret[r.url.Host] = keys
	}

	if multipleErrors != nil {
		return nil, multipleErrors
	}

	return ret, nil
}

// DeleteKey deletes an encryption key
func (c *Client) DeleteKey(ctx context.Context, kid string, opts ...Options) error {
	// let's make sure we can reach all the servers
	if _, err := c.Health(ctx); err != nil {
		return err
	}

	// get keys
	getResponses := c.doP(ctx, "GET", constant.KeyAPIPrefix, c.servers, modifiers(opts...)...)
	if err := handleErrors(getResponses); err != nil {
		return err
	}

	allKeys, err := getKeys(getResponses)
	if err != nil {
		return err
	}

	var lastKeyErr error
	exist := false
	for server, keys := range allKeys {
		for _, key := range keys {
			if strings.HasPrefix(key.Signature, kid) {
				exist = true
				if len(keys) == 1 {
					lastKeyErr = errors.Join(lastKeyErr, fmt.Errorf("last key cannot be deleted on %s", server))
				}
				kid = key.Signature
				break
			}
		}
	}
	if lastKeyErr != nil {
		return errors.Join(lastKeyErr, hos.ErrNotAllowed)
	}
	if !exist {
		return fmt.Errorf("key %s %w", kid, hos.ErrNotExist)
	}

	deleteResponses := c.doP(ctx, "DELETE", path.Join(constant.KeyAPIPrefix, kid), c.servers, modifiers(opts...)...)
	errorHandlerOptions := append(errHandlers(opts...), WarnErrors(hos.ErrNotExist))
	return handleErrors(deleteResponses, errorHandlerOptions...)
}
