// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
	"github.com/spf13/cobra"
)

func newAttrCmd() *cobra.Command {
	var useID bool

	cmd := &cobra.Command{
		Use:     "attr",
		Aliases: []string{"attribute"},
		Short:   "add pool attributes",
		Long: `add attributes to a pool
attributes can be any arbitrary data
attributes cannot be altered or deleted after they are added

Examples:
#add attributes to a pool by id
hos attr --id f1658489 Sync='{PoolID:"a7b376c8"}'

#add attributes to a pool by name
hos attr Docs color=green
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 2 {
				return fmt.Errorf(`hos attr POOL KEY=VALUE...
expected a pool and at least a key=val as arguments, got only %d args`, argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			attributes := map[string]string{}
			for _, attr := range args[1:] {
				key, val, err := validate.ParseAttr(attr)
				if err != nil {
					return err
				}
				attributes[key] = val
			}

			result, err := parseArg(userID, args[0], &argFlags{id: useID, pool: true})
			if err != nil {
				return err
			}

			// we get the pool here to check attrs
			// server makes the same check but pools might be out of sync
			pool, err := hosClient.GetPool(cmd.Context(), result.poolID, clientOptions...)
			if err != nil {
				return err
			}

			for key := range pool.Attributes {
				if _, ok := attributes[key]; ok {
					return fmt.Errorf("attr %s is already exist, attributes cannot be changed", key)
				}
			}
			editPool := &hos.Pool{ID: pool.ID, Attributes: attributes}

			_, err = hosClient.EditPool(cmd.Context(), editPool, clientOptions...)
			return err
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use dest pool id instead of its name")

	return cmd
}
