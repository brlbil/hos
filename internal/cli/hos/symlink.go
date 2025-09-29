// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/brlbil/hos"
	"github.com/spf13/cobra"
)

func newSymlinkCmd() *cobra.Command {
	var (
		useID bool
		dir   string
		pools []hos.Pool
	)

	cmd := &cobra.Command{
		Use:   "symlink",
		Short: "create symbolic links to objects",
		Long: `creates symbolic links to objects that reside on the same server
this command only works with a local cluster

creates directories for pools and for objects that have names with paths like Images/2005/img1.jpg
if destination directory does not exist, it is created

if --id flag is set, pools' IDs are expected instead of their names

Examples:
#create symlinks for pools under Test directory
hos exp symlink Images Docs Test
`,

		Args: func(_ *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength == 0 {
				return fmt.Errorf("hos exp symlink POOL... DIR\nexpected destination directory path as the last argument, got 0 args")
			} else if argsLength < 2 {
				return fmt.Errorf("hos exp symlink POOL... DIR\nexpected at least one pool name as an argument, got 1 args")
			}
			return nil
		},

		PreRunE: func(cmd *cobra.Command, args []string) error {
			argsLen := len(args)
			for _, arg := range args[0 : argsLen-1] {
				result, err := parseArg(userID, arg, &argFlags{id: useID, pool: true})
				if err != nil {
					return err
				}
				pool, err := hosClient.GetPool(cmd.Context(), result.poolID)
				if err != nil {
					return err
				}
				pools = append(pools, *pool)
			}

			dir = args[argsLen-1]
			if stat, err := os.Stat(dir); err == nil && !stat.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}

			return nil
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			// get server configuration
			serverConfMap, err := hosClient.ServerConfig(cmd.Context())
			if err != nil {
				return err
			}

			for _, pool := range pools {
				poolDir := filepath.Join(dir, pool.Name)
				// remove everything, this is easier instead of trying to sync for changes
				if err := os.RemoveAll(poolDir); err != nil {
					return err
				}

				if err := os.MkdirAll(poolDir, 0o755); err != nil {
					return err
				}

				objects, err := hosClient.ListObjects(cmd.Context(), pool.ID)
				if err != nil {
					return err
				}
				if len(objects) == 0 {
					continue
				}

				for _, object := range objects {
					objectDir := path.Dir(object.Name)
					if objectDir != "." {
						if err := os.MkdirAll(filepath.Join(dir, pool.Name, objectDir), 0o755); err != nil {
							return err
						}
					}

					serverConf, ok := serverConfMap[object.ServerAddr()]
					if !ok {
						fmt.Fprintf(os.Stderr, "server %s configuration is not found, obj: name=%s id=%s", object.ServerAddr(), object.Name, object.ID)
						continue
					}

					if err := os.Symlink(
						filepath.Join(serverConf.RootDir, pool.ID, object.ID),
						filepath.Join(dir, pool.Name, object.Name),
					); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&useID, "id", false, "use argument as pool ID instead of its name")

	return cmd
}
