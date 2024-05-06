package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// set by makefile with ldflags
var (
	Version = "dev"
	Commit  = "n/a"
	Build   = "n/a"
)

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Application version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(Version)
			fmt.Println("Commit:", Commit)
			fmt.Println("Build:", Build)
		},
	}
	return cmd
}
