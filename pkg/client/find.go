// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"path"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/constant"
)

// Find searches Pools and Objects with fuzzy matching their names and search text
// returns Objects with only Name, ID and PoolID is populated
// for Pools PoolID is returned empty
func (c *Client) Find(ctx context.Context, text string, opts ...Options) ([]hos.Object, error) {
	if text == "" {
		return nil, fmt.Errorf("searh text cannot be empty %w", hos.ErrBadRequest)
	}

	mod := append(modifiers(opts...), urlQueries(map[string]string{"name": text}))
	rsp := c.doP(ctx, "GET", path.Join(constant.APIPrefix, "find"), c.servers, mod...)

	if err := handleErrors(rsp, errHandlers(opts...)...); err != nil {
		return nil, err
	}

	flt := append(filters(opts...), &sortByName{})
	oo, err := merge[hos.Object](rsp, flt...)
	if err != nil {
		return nil, err
	}

	return oo, nil
}
