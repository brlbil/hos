// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/brlbil/hos"
	"github.com/spf13/cobra"
)

func newLinkPoolCmd() *cobra.Command {
	var useID bool

	cmd := &cobra.Command{
		Use:     "link",
		Aliases: []string{"ln"},
		Short:   "create a linked pool",
		Long: `create a pool linked to another pool

other users' pools can be linked, but can only be accessed
if the user has permission on the destination pool

Examples:
#create a linked pool with id
hos link --id 71f0cd3e LinkedImages

#create a linked pool as an alias to another
hos link Images LinkedImages

#create a linked pool to another user's pool
hos link user1@Images LinkedImages

#create a linked pool to another user's pool with the same name
hos link user1@Images
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 1 {
				return fmt.Errorf("hos ln SRC_POOL\nexpected a source pool as an argument, got %d args", argsLength)
			}
			if argsLength := len(args); argsLength > 2 {
				return fmt.Errorf("hos ln SRC_POOL LINKED_POOL\nexpected a source pool and linked pool as arguments, got %d args", argsLength)
			}
			if len(args) == 1 && useID {
				return errors.New(`hos ln --id SRC_POOL_ID LINKED_POOL
--id flag set, exactly 2 args are required, source pool ID and linked pool name, got 1 arg`)
			}
			hasUserPrefix := strings.ContainsAny(args[0], "@")
			if !useID && !hasUserPrefix && len(args) == 1 {
				return errors.New(`hos SRC_POOL LINKED_POOL
linking self-owned pool, linked and source pool name cannot be the same,
required second arg POOL_NAME`)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			hasUserPrefix := strings.ContainsAny(args[0], "@")
			result, err := parseArg(userID, args[0], &argFlags{id: useID, pool: true, userAt: hasUserPrefix})
			if err != nil {
				return err
			}

			destinationPoolName := result.poolName
			if len(args) == 2 {
				if _, err := parsePoolArg(userID, args[1], false); err != nil {
					return err
				}
				destinationPoolName = args[1]
			}

			_, err = hosClient.CreatePool(
				cmd.Context(),
				&hos.Pool{
					Name:     destinationPoolName,
					LinkedID: result.poolID,
				},
				clientOptions...,
			)

			return err
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use first argument as pool ID instead of its name")

	return cmd
}
