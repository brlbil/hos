// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/pkg/client"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var (
		force     bool
		useID     bool
		recursive bool
		labels    []string
	)

	cmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"rm"},
		Short:   "delete pool and object(s)",
		Long: `delete a pool and objects
only an empty pool can be deleted, to delete a pool with objects run with -R (--recursive) flag
this will delete all the objects and the pool

an object can be deleted with its full path

to delete multiple objects, they can be selected by using OBJECT_PREFIX... notation
or by their labels with -l (--label-selector) flag

if objects do not have all their copies, deletion is prevented
to force it, run with -f (--force) flag

Examples:
#delete a pool with ID
hos remove --id e7b3bb5c

#delete a pool recursively
hos rm -R Images

#delete objects matching label selector
hos rm Images -l k=v,j!=u

#delete an object with ID
hos rm --id 5a52931a/684cb32c

#delete multiple objects
hos rm Images/Holidays/2011/...

#delete multiple objects that start with Holiday/2011/ and match the label selector
hos rm Images/Holidays/2011/... -l k==v
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf(`hos rm {POOL, POOL/OBJECT, POOL/OBJECT...}
expected a pool or pool/object(...) as an argument, got %d args`, argsLength)
			}
			return nil
		},

		PreRunE: func(_ *cobra.Command, _ []string) error {
			if recursive && len(labels) > 0 {
				return errors.New("-R (recursive) and -l (label-selector) flags cannot be used at the same time")
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parseArg(userID, args[0], &argFlags{recursive: recursive, id: useID, poolObj: true, labels: labels})
			if err != nil {
				return err
			}

			if result.objID != "" {
				if force {
					clientOptions = append(clientOptions, client.IgnoreErrors(hos.ErrCorrupted, hos.ErrNotAllCopiesAvailable, hos.ErrNotExist))
				}
				return hosClient.DeleteObject(cmd.Context(), result.poolID, result.objID, clientOptions...)
			}

			if len(result.options) > 0 {
				options := append(result.options, clientOptions...)
				options = append(options, client.IgnoreErrors(hos.ErrNotExist))
				objects, err := hosClient.ListObjects(cmd.Context(), result.poolID, options...)
				if err != nil {
					return err
				}

				if len(objects) == 0 {
					fmt.Println("no objects deleted")
					return nil
				}

				if force {
					clientOptions = append(clientOptions, client.IgnoreErrors(hos.ErrCorrupted, hos.ErrNotAllCopiesAvailable, hos.ErrNotExist))
				}
				for _, object := range objects {
					if err := hosClient.DeleteObject(cmd.Context(), object.PoolID, object.ID, clientOptions...); err != nil {
						return err
					}
				}

				if !recursive {
					return nil
				}
			}

			return hosClient.DeletePool(cmd.Context(), result.poolID, clientOptions...)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "", false, "force deletion of object even if not all the copies are available")
	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "delete the pool recursively")
	cmd.Flags().StringArrayVarP(&labels, "label-selector", "l", []string{},
		"label selector to filter on, supports '==' and '!=' (e.g. -l key1==value1,key2!=value2)")

	return cmd
}
