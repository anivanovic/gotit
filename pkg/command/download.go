package command

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/download"
	"github.com/anivanovic/gotit/pkg/torrent"
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

		Run: func(cmd *cobra.Command, args []string) {
			err := runDownload(cmd, args, f)
			if err != nil {
				log.Fatal("got error:", err)
				return
			}
		},
	}
	cmd.Flags().StringVarP(&f.output, "out", "o", "", "Torrent download output directory")
	cmd.Flags().IntVarP(&f.peerNum, "num-peer", "n", 30, "Maximum number of peers to download torrent from")
	cmd.Flags().IntVarP(&f.listenPort, "port", "p", 6666, "Port number on which to listen for other peers requests")
	cmd.MarkFlagRequired("out")

	return cmd
}

func runDownload(_ *cobra.Command, args []string, f *flags) error {
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

	torrent, err := torrent.New(&torrentMetadata, outDir)
	if err != nil {
		return err
	}
	mng := download.NewMng(torrent, f.peerNum, f.listenPort)
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs

		mng.Close()
	}()

	if err := mng.Download(); err != nil {
		return err
	}

	return nil
}
