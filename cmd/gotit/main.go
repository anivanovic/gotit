package main

import (
	"os"

	"github.com/anivanovic/gotit/pkg/cmd"
)

type config struct {
	logLevel  string
	logFormat string
	cfgPath   string
}

func main() {
	app := cmd.NewApp()
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}
