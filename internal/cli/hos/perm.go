// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
	"github.com/spf13/cobra"
)

func newPermCmd() *cobra.Command {
	var (
		useID bool
		rm    bool
	)

	cmd := &cobra.Command{
		Use:     "perm",
		Aliases: []string{"permission"},
		Short:   "edit permissions",
		Long: `permissions are defined only for pools
expected arguments: a pool name or ID with --id flag and one or more permissions

permissions are defined as <user name> or '*' (for every user) : {r(read), w(write)}, write permission implies read as well
giving a user=permission that does not exist adds it, giving an existing one modifies it

with --rm option only <user name> or '*' is accepted, permission associated with it is removed

Examples:
#add and/or modify permissions with ID
hos perm --id f1658489 '*:r' user1:w

#add and/or change permissions
hos perm Test '*:w' user2:r

#remove permissions
hos perm Docs --rm '*' user1
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 2 {
				return fmt.Errorf("hos perm POOL PERM...\nexpected a pool and one or more permissions as arguments, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			perms := map[string]hos.Permission{}
			if rm {
				for _, perm := range args[1:] {
					if err := validate.PermSelector(perm); err != nil {
						return err
					}
					perms["!"+perm] = "r"
				}
			} else {
				for _, perm := range args[1:] {
					key, permission, err := validate.ParsePerm(perm)
					if err != nil {
						return err
					}
					perms[key] = permission
				}
			}

			result, err := parseArg(userID, args[0], &argFlags{id: useID, pool: true})
			if err != nil {
				return err
			}

			pool := &hos.Pool{
				ID:          result.poolID,
				Permissions: perms,
			}

			_, err = hosClient.EditPool(cmd.Context(), pool, clientOptions...)
			return err
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool ID instead of its name")
	cmd.Flags().BoolVar(&rm, "rm", false, "remove permissions")

	return cmd
}
