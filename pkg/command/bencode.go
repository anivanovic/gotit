package command

import (
	"fmt"
	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/gotit"
	"github.com/spf13/cobra"
	"io"
	"os"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bencode <file>",
		Short: "Inspect torrent file",
		Long:  "Decode torrent file in bencode format and print human readable metadata about torrent",
		Args:  cobra.ExactArgs(1),
		Run:   run,
	}
	return cmd
}

func run(_ *cobra.Command, args []string) {
	file := args[0]
	f, err := os.Open(file)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	torrent := gotit.TorrentMetadata{}
	if err := bencode.Unmarshal(data, &torrent); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	torrent2, err := gotit.NewTorrent(&torrent, "")
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(torrent2)
}
