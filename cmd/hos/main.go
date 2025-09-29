// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"

	cmd "github.com/brlbil/hos/internal/cli/hos"
)

func main() {
	if err := cmd.NewHosClientCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
