package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/anivanovic/gotit/pkg/command"
)

func main() {
	rootCmd := cobra.Command{Use: "gotit"}
	rootCmd.AddCommand(command.NewCommand())
	rootCmd.AddCommand(command.NewDownloadCommand())
	rootCmd.AddCommand(command.NewVersionCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
