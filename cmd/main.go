package main

import (
	"github.com/anivanovic/gotit/pkg/command"
	"github.com/spf13/cobra"
	"os"
)

func main() {
	rootCmd := cobra.Command{Use: "gotit"}
	rootCmd.AddCommand(command.NewCommand())
	rootCmd.AddCommand(command.NewVersionCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
