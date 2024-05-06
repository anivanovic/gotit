package cmd

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
)

func NewCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bencode <file>",
		Short: "Inspect torrent file",
		Long:  "Decode torrent file in bencode format and print human readable metadata about torrent",
		Args:  cobra.ExactArgs(1),
		Run:   app.NewCmdRun(run),
	}
	return cmd
}

func run(_ context.Context, appContext AppContext, args []string) error {
	file := args[0]
	f, err := os.Open(file)
	if err != nil {
		appContext.log.Fatal("open file", zap.Error(err))
	}

	data, err := io.ReadAll(f)
	if err != nil {
		appContext.log.Sugar().Fatalf("read file %s: %v", file, err)
	}

	torrentMetadata := bencode.Metainfo{}
	if err := bencode.Unmarshal(data, &torrentMetadata); err != nil {
		appContext.log.Fatal("parse torrent file", zap.Error(err))
	}

	//appContext.printer.Info("Metainfo file:", file)
	//appContext.printer.Info(torrentMetadata)

	// TODO: return errors instead
	return nil
}
