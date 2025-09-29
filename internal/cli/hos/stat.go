// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos/internal/out"
	"github.com/spf13/cobra"
)

func newStatCmd() *cobra.Command {
	var (
		useID  bool
		output outType = "wide"
	)

	cmd := &cobra.Command{
		Use:   "stat",
		Short: "display user stats, pool or object information",
		Long: `display user statistics with no arguments
display a single pool or object information with their names or IDs as the argument

output format can be set with -o (output) flag, possible values (json, yaml, name, fields)

Examples:
#display user usage information 
hos stat

#display a pool info with its ID
hos stat --id 237hdn4b

#display a pool info
hos stat Shows

#display an object info with its ID
hos stat --id 237hdn4b/4da8hb13

#display an object info
hos stat Shows/Wonder.avi
`,
		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength > 1 {
				return fmt.Errorf("hos ls [POOL, POOL/OBJECT]\nexpected no arguments, or a pool, pool/object as an argument, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				usage, err := hosClient.GetUsage(cmd.Context(), clientOptions...)
				if err != nil {
					return err
				}

				return out.Print(usage, output.String())
			}

			result, err := parseArg(userID, args[0], &argFlags{id: useID, poolObj: true})
			if err != nil {
				return err
			}

			if result.objID == "" && len(result.options) > 0 {
				return fmt.Errorf("only one pool or object supported, arg %s is not a valid pool or pool/object name|ID", args[0])
			}

			if result.objID == "" {
				pool, err := hosClient.GetPool(cmd.Context(), result.poolID, clientOptions...)
				if err != nil {
					return err
				}

				return out.Print(pool, output.String())
			}

			object, err := hosClient.GetObject(cmd.Context(), result.poolID, result.objID, clientOptions...)
			if err != nil {
				return err
			}

			return out.Print(object, output.String())
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")
	cmd.Flags().VarP(&output, "output", "o", "output format, one of: (json, yaml, name, fields)")

	return cmd
}
