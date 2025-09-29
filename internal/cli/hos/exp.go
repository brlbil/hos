// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/spf13/cobra"
)

func newExpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exp",
		Short: "experimental features",
		Long: `experimental commands

commands under experimental are subject to change or be dropped entirely
`,
	}

	cmd.AddCommand(
		newHTMLCmd(),
		newMountCmd(),
		newSymlinkCmd(),
	)

	return cmd
}
