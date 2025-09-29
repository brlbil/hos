// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/brlbil/hos/internal/html"
	"github.com/brlbil/hos/internal/service"
	"github.com/spf13/cobra"
)

var (
	htmlAddr     string
	htmlLogLevel string
)

func newHTMLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "html",
		Short: "serve pools and objects as HTML file listing",
		Long: `starts a web server on the specified address and port
server lists pools and objects as an HTML file list`,

		RunE: func(_ *cobra.Command, _ []string) error {
			htmlService, err := html.New(htmlAddr, htmlLogLevel, hosClient, cmdFlags.impersonateUser)
			if err != nil {
				return err
			}
			fmt.Println("Serving at", "http://"+htmlAddr)

			return service.Run(htmlService)
		},
	}

	cmd.Flags().StringVarP(&htmlAddr, "address", "a", "localhost:8998", "Server address to listen on")
	cmd.Flags().StringVarP(&htmlLogLevel, "log", "l", "error", "Logging level")

	return cmd
}
