package main

import (
	"github.com/anivanovic/gotit/pkg/command"
	"github.com/anivanovic/gotit/pkg/gotit"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var log *zap.Logger

// set up logger
func initLogger() {
	l := zapcore.InfoLevel

	cfg := zap.NewProductionConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	cfg.Level = zap.NewAtomicLevelAt(l)
	log, _ = cfg.Build()

	zap.ReplaceGlobals(log)
	gotit.SetLogger(log)
}

func main() {
	initLogger()
	defer log.Sync()

	rootCmd := cobra.Command{Use: "gotit"}
	rootCmd.AddCommand(command.NewCommand())
	rootCmd.AddCommand(command.NewDownloadCommand())
	rootCmd.AddCommand(command.NewVersionCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
