// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strings"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/out"
	"github.com/brlbil/hos/pkg/client"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var (
		useID  bool
		labels []string
		output outType = "default"
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list pools or object(s)",
		Long: `list pools with no arguments

list all objects in a pool with pool name or ID as an argument
listing objects can be filtered by using OBJECT_PREFIX... notation

list one object with full pool name, object name or IDs as an argument

pools and objects can be filtered with -l (label-selector) flag e.g. -l key==val,key1!=val1

output format can be set with -o (output) flag, possible values (json, yaml, name, fields)

Examples:
#list pools
hos list

#list objects with pool ID
hos ls --id 2041f02b

#list all objects
hos ls Documents

#list objects whose names start with Sun
hos ls Documents/Sun...

#list objects whose names start with Sun and have label k=v
hos ls Documents/Sun... -l k==v

#list objects that have label k=v
hos ls Documents -l k==v

#list an object with pool/object ID
hos ls --id 2041f02b/0032c42d

#list an object with pool/object name
hos ls Documents/Sunny
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength > 1 {
				return fmt.Errorf(`hos ls [POOL, POOL/OBJECT, POOL/OBJECT...]
expected no arguments or a pool, a pool/object(...) as an argument, got only %d args`, argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				labelOptions, err := parseLabels(labels)
				if err != nil {
					return err
				}
				if len(labelOptions) > 0 {
					clientOptions = append(clientOptions, labelOptions...)
				}
				clientOptions = append(clientOptions, client.WarnErrors(hos.ErrNotExist))
				pools, err := hosClient.ListPools(cmd.Context(), clientOptions...)
				if err != nil {
					return err
				}

				return out.Print(pools, output.String())
			}

			recursive := !strings.ContainsAny(args[0], "/")
			result, err := parseArg(userID, args[0], &argFlags{id: useID, recursive: recursive, labels: labels, poolObj: true})
			if err != nil {
				return err
			}

			clientOptions = append(clientOptions, ignoredErrorOptions...)
			if result.objID != "" {
				object, err := hosClient.GetObject(cmd.Context(), result.poolID, result.objID, clientOptions...)
				if err != nil {
					return err
				}
				return out.Print(object, output.String())
			}
			result.options = append(result.options, clientOptions...)
			objects, err := hosClient.ListObjects(cmd.Context(), result.poolID, result.options...)
			if err != nil {
				return err
			}

			return out.Print(objects, output.String())
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool/object ID instead of its name")
	cmd.Flags().StringArrayVarP(&labels, "label-selector", "l", []string{},
		"label selector to filter on, supports '==' and '!=' (e.g. -l key1==value1,key2!=value2)")
	cmd.Flags().VarP(&output, "output", "o", "output format, one of: (json, yaml, name, fields)")

	return cmd
}
