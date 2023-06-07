package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	_ "net/http/pprof"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/download"
	"github.com/anivanovic/gotit/pkg/torrent"
	"github.com/anivanovic/gotit/pkg/util"
)

type flags struct {
	output     string
	peerNum    int
	listenPort int
}

func newFlags() *flags {
	return &flags{}
}

func NewDownloadCommand() *cobra.Command {
	f := newFlags()
	cmd := &cobra.Command{
		Use:   "download -out <out_dir> <torrent_file>",
		Short: "Download torrent",
		Long:  "Start download process for torrent file",
		Args:  cobra.ExactArgs(1),

		Run: util.NewCmdRun(func(ctx context.Context, app util.ApplicationContainer, args []string) error {
			return runDownload(ctx, app.Logger, args, f)
		}),
	}
	cmd.Flags().StringVarP(&f.output, "out", "o", "", "Torrent download output directory")
	cmd.Flags().IntVarP(&f.peerNum, "num-peer", "n", 30, "Maximum number of peers to download torrent from")
	cmd.Flags().IntVarP(&f.listenPort, "port", "p", 6666, "Port number on which to listen for other peers requests")
	cmd.MarkFlagRequired("out")

	return cmd
}

func runDownload(ctx context.Context, l *zap.Logger, args []string, f *flags) error {
	outDir := f.output
	torrentFile := args[0]
	stat, err := os.Stat(outDir)
	if err != nil {
		return err
	}

	if !stat.IsDir() {
		return errors.New(fmt.Sprintf("not a directory: %s", outDir))
	}

	file, err := os.Open(torrentFile)
	if err != nil {
		return err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	var torrentMetadata bencode.Metainfo
	if err := bencode.Unmarshal(data, &torrentMetadata); err != nil {
		return err
	}

	t, err := torrent.New(&torrentMetadata, outDir, l)
	if err != nil {
		return err
	}
	mng := download.NewMng(t, l, f.peerNum, f.listenPort)
	defer mng.Stop()

	return mng.Download(ctx)
}
