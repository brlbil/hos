// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos/internal/out"
	"github.com/spf13/cobra"
)

type object struct {
	Pool string `print:"default"`
	ID   string `print:"default"`
	Name string `print:"default"`
}

func newFindCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "find",
		Aliases: []string{"f"},
		Short:   "find pools and objects by name",
		Long: `find makes fuzzy matches for pool and object names
returning ID, name for pools and objects, and pool=true for pools only

Examples:
#fuzzy find 'abc'
hos find abc
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf("hos find SEARCH_TEXT\nexpected only one search text argument, got %d args", argsLength)
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := hosClient.Find(cmd.Context(), args[0], clientOptions...)
			if err != nil {
				fmt.Printf("err: %s", err)
				return err
			}

			objects := []object{}
			for _, result := range results {
				poolIndicator := "x"
				if result.PoolID != "" {
					poolIndicator = ""
				}
				objects = append(objects, object{Name: result.Name, ID: result.ID, Pool: poolIndicator})
			}

			return out.Print(objects, "default")
		},
	}

	return cmd
}
