// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos"
	"github.com/brlbil/hos/internal/validate"
	"github.com/spf13/cobra"
)

func newMakePoolCmd() *cobra.Command {
	var (
		encrypted    bool
		replicaCount uint
		attributes   map[string]string
		labels       map[string]string
	)

	cmd := &cobra.Command{
		Use:     "make-pool",
		Aliases: []string{"mp", "mkp"},
		Short:   "create pool",
		Long: `create a pool on all available servers in the cluster

replica count can be set with -r (--replica-count) flag, the default replica count is 1

labels can be set on the pool with -l (--label) flag

attributes can be set on the pool with -a (--attribute) flag
attributes add behavior-specific metadata, cannot be changed later

pool can be encrypted with -e (--encrypted) flag

Examples:
#create a pool with replication count of 2
hos make-pool Images -r 2

#create a pool with key=value label and default replica count
hos mkp Images -l key=value
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength != 1 {
				return fmt.Errorf("hos make-pool POOL\nexpected a pool name as an argument, got %d args", argsLength)
			}
			return nil
		},

		PreRunE: func(_ *cobra.Command, _ []string) error {
			for key, val := range labels {
				if err := validate.Label(key); err != nil {
					return err
				}
				if err := validate.LabelValue(val); err != nil {
					return err
				}
			}
			for key, val := range attributes {
				if _, _, err := validate.ParseAttr(key + "=" + val); err != nil {
					return err
				}
			}
			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			pool := &hos.Pool{
				Name:         args[0],
				ReplicaCount: int(replicaCount),
				Encrypted:    encrypted,
			}

			if len(labels) > 0 {
				pool.Labels = labels
			}
			if len(attributes) > 0 {
				pool.Attributes = attributes
			}

			_, err := hosClient.CreatePool(cmd.Context(), pool, clientOptions...)

			return err
		},
	}

	cmd.Flags().UintVarP(&replicaCount, "replica-count", "r", 1, "replication count")
	cmd.Flags().StringToStringVarP(&labels, "label", "l", map[string]string{}, "add labels")
	cmd.Flags().StringToStringVarP(&attributes, "attribute", "a", map[string]string{}, "add attributes")
	cmd.Flags().BoolVarP(&encrypted, "encrypted", "e", false, "set encrypted attribute, objects created under the pool will be encrypted")

	return cmd
}
