package command

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/anivanovic/gotit/pkg/bencode"
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
		fmt.Fprintf(os.Stderr, "could not open %s: %v", file, err)
		os.Exit(1)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v", file, err)
		os.Exit(1)
	}

	torrentMetadata := bencode.Metainfo{}
	if err := bencode.Unmarshal(data, &torrentMetadata); err != nil {
		fmt.Fprintf(os.Stderr, "parsing metadata file: %v", err)
		os.Exit(1)
	}

	fmt.Println("Metainfo file:", file)
	fmt.Println(torrentMetadata)
}
