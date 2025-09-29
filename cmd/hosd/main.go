// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"

	cmd "github.com/brlbil/hos/internal/cli/hosd"
)

func main() {
	if err := cmd.NewHosServerCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
