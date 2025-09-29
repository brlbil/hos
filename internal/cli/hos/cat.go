// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newCatCmd() *cobra.Command {
	var useID bool

	cmd := &cobra.Command{
		Use:   "cat",
		Short: "print object content",
		Long: `print content of an object to stdout
fails if the object's content is not printable

Examples:
#print an object's content with its ID
hos cat --id 237hdn4b/4da8hb13

#print an object's content 
hos cat Docs/list.txt
`,
		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf("hos cat POOL/OBJECT\nexpected a pool/object as an argument, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := parseArg(userID, args[0], &argFlags{id: useID})
			if err != nil {
				return err
			}

			if result.objID == "" && len(result.options) > 0 {
				return fmt.Errorf("only one object supported, arg %s is not a valid pool_name/object_name", args[0])
			}

			objectContent, err := hosClient.GetContent(cmd.Context(), result.poolID, result.objID, clientOptions...)
			if err != nil {
				return err
			}
			defer objectContent.Close()

			_, err = io.CopyN(os.Stdout, objectContent, objectContent.Size)
			if err != nil {
				return fmt.Errorf("copy error: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")

	return cmd
}
