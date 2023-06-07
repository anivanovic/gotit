package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/util"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bencode <file>",
		Short: "Inspect torrent file",
		Long:  "Decode torrent file in bencode format and print human readable metadata about torrent",
		Args:  cobra.ExactArgs(1),
		Run:   util.NewCmdRun(run),
	}
	return cmd
}

func run(_ context.Context, app util.ApplicationContainer, args []string) error {
	file := args[0]
	f, err := os.Open(file)
	if err != nil {
		app.Logger.Fatal("open file", zap.Error(err))
	}

	data, err := io.ReadAll(f)
	if err != nil {
		app.Logger.Sugar().Fatalf("read file %s: %v", file, err)
	}

	torrentMetadata := bencode.Metainfo{}
	if err := bencode.Unmarshal(data, &torrentMetadata); err != nil {
		app.Logger.Fatal("parse torrent file", zap.Error(err))
	}

	fmt.Println("Metainfo file:", file)
	fmt.Println(torrentMetadata)

	// TODO: return errors instead
	return nil
}
