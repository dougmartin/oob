package main

import (
	"fmt"
	"os"

	"github.com/dougmartin/oob/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "oob:", err)
		os.Exit(1)
	}
}
