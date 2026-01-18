package main

import (
	"fmt"
	"os"

	"github.com/lucasefe/seedup/internal/cli"
)

func main() {
	rootCmd := cli.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
