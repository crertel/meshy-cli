package main

import (
	"fmt"
	"os"

	"github.com/crertel/meshy-cli/internal/cli"
)

func main() {
	root := cli.NewRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
