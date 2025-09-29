// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos"
	"github.com/spf13/cobra"
)

func newMoveCmd() *cobra.Command {
	var (
		useID  bool
		labels []string
	)

	cmd := &cobra.Command{
		Use:     "move",
		Aliases: []string{"mv"},
		Short:   "move object(s)",
		Long: `move one or more objects from one pool to another pool
move an object within the same pool to rename it
source and destination pool owners, replication count and encryption type must match

Examples:
#move an object to another pool
hos move Images/image.jpeg Pictures

#move an object to another pool with ID
hos move e7b3bb5c/67a1c763 760a4d96

#rename an object
hos move Images/image.jpeg Images/image1.jpeg

#move multiple objects whose names start with /images/2001/ to another pool
hos move Images/images/2001/... Pictures

#move multiple objects that have label k=v to another pool
hos move Images Pictures -l k==v
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 2 {
				return fmt.Errorf(`hos mv {POOL, POOL/OBJECT, POOL/OBJECT...} {POOL, POOL/OBJECT}
expected a pool/object(...) and a pool or pool/object as arguments, got %d args`, argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			hasLabels := len(labels) > 0
			sourceResult, err := parseArg(userID, args[0], &argFlags{id: useID, poolObj: hasLabels, labels: labels})
			if err != nil {
				return err
			}

			destinationResult, err := parseArg(userID, args[1], &argFlags{id: useID, pool: useID, poolObj: !useID})
			if err != nil {
				return err
			}

			if len(destinationResult.options) > 0 {
				return fmt.Errorf("arg %s is not a valid pool/object name", args[1])
			}

			var objects []hos.Object
			if len(sourceResult.options) > 0 {
				if destinationResult.objPath != "" {
					return fmt.Errorf("dest must be a pool when moving multiple objects, %s is not a valid pool name", args[1])
				}

				objects, err = hosClient.ListObjects(cmd.Context(), sourceResult.poolID, append(sourceResult.options, clientOptions...)...)
				if err != nil {
					return err
				}
			} else {
				objects = []hos.Object{{ID: sourceResult.objID, PoolID: sourceResult.poolID}}
			}

			for _, object := range objects {
				err := hosClient.MoveObject(
					cmd.Context(),
					object.PoolID,
					object.ID,
					destinationResult.poolID,
					destinationResult.objPath,
					clientOptions...)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")
	cmd.Flags().StringArrayVarP(&labels, "label-selector", "l", []string{},
		"label selector to filter on, supports '==' and '!=' (e.g. -l key1==value1,key2!=value2)")

	return cmd
}
