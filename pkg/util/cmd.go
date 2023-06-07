package util

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/logger"
)

type CobraRunFn func(cmd *cobra.Command, args []string)

type ApplicationContainer struct {
	Logger *zap.Logger
}

func NewCmdRun(fn func(ctx context.Context, app ApplicationContainer, args []string) error) CobraRunFn {
	ctx := context.Background()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)

	l, err := logger.NewCommandLine("debug", "color")
	if err != nil {
		l.Fatal("could not run", zap.Error(err))
	}
	app := ApplicationContainer{Logger: l}
	return func(_ *cobra.Command, args []string) {
		defer stop()
		if err := fn(ctx, app, args); err != nil {
			l.Fatal("could not run", zap.Error(err))
		}
	}
}
