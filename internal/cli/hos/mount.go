// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos/internal/hosfs"
	"github.com/spf13/cobra"
)

func newMountCmd() *cobra.Command {
	var (
		useID         bool
		mountLogLevel string
		fuseDebugLog  bool
		pools         []string
	)

	cmd := &cobra.Command{
		Use:   "mount",
		Short: "mount FUSE file system",
		Long: `mount one or more pools as a read-only FUSE file system

with no options, all the pools are mounted as root level directories
with pool names as arguments, only given pools are mounted as root level directories
with only one pool name argument, objects are mounted as root level directories or files

if --id flag is set, pools' IDs are expected instead of their names

Examples:
#mount all pools under test directory
hos mount test/

#mount two pools under Sel dir
hos mount Images Docs Sel/

#mount only one pool under Images dir
hos mount Images Images/
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 1 {
				return fmt.Errorf("hos mount POOL... MOUNT_DIR\nexpected at least a mount directory as an argument, got %d args", argsLength)
			}
			return nil
		},

		PreRunE: func(_ *cobra.Command, args []string) error {
			for _, arg := range args[:len(args)-1] {
				result, err := parseArg(userID, arg, &argFlags{id: useID, pool: true})
				if err != nil {
					return err
				}
				pools = append(pools, result.poolID)
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			opts := []hosfs.ConfigFunc{
				hosfs.SetLogging(mountLogLevel),
				hosfs.SetPoolID(pools...),
				hosfs.SetImpersonatedUser(cmdFlags.impersonateUser),
			}
			if fuseDebugLog {
				opts = append(opts, hosfs.EnableFuseDebugLog)
			}
			return hosfs.Mount(cmd.Context(), args[len(args)-1], hosClient, opts...)
		},
	}

	cmd.Flags().StringVarP(&mountLogLevel, "log", "l", "none", "logging level")
	cmd.Flags().BoolVarP(&fuseDebugLog, "enable-fuse-debug", "D", false, "enable fuse debug logging")
	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool ID instead of its name")

	return cmd
}
