// SPDX-License-Identifier: MIT

// Package cmd provides the HOS server CLI implementation.
// It handles the hosd command for starting and configuring HOS servers.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/brlbil/hos/internal/service"
	"github.com/brlbil/hos/pkg/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	vp     = newWiper()
	config = server.Config{}
)

func NewHosServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hosd",
		Short: "Home Object Storage server daemon",
		Long: `HOS server daemon that serves REST API to manage pools, objects, and users.
PATH specifies the data directory where HOS will store pools, objects, and metadata.

Examples:
#start server with current directory as data directory
hosd .

#start server with specific data directory
hosd /var/lib/hos

#start server with custom address and log level
hosd /var/lib/hos --address :8080 --log debug
`,
		SilenceErrors: true,
		SilenceUsage:  true,

		PreRunE: func(cmd *cobra.Command, args []string) error {
			if argsLength := len(args); argsLength < 1 {
				return fmt.Errorf("hosd [flags] DATA_PATH\nexpected one data path as an argument, got %d args", argsLength)
			}
			flags := cmd.Flags()
			return vp.BindPFlags(flags)
		},

		RunE: func(_ *cobra.Command, args []string) error {
			config.RootDir = args[0]

			s, err := server.New(&config)
			if err != nil {
				return fmt.Errorf("NewServer failed: %w", err)
			}

			return service.Run(s)
		},
	}

	cmd.Flags().StringVarP(&config.LogLevel, "log", "l", "info", "logging level (debug, info, warn, error)")
	cmd.Flags().StringVarP(&config.Address, "address", "a", "0.0.0.0:1981", "address and port to listen on")

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	return cmd
}

func newWiper() *viper.Viper {
	vp := viper.New()
	vp.SetEnvPrefix("HOSD")
	vp.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	vp.AutomaticEnv()

	return vp
}
