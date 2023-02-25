package gotit

import (
	"context"
	"github.com/anivanovic/gotit/pkg/torrent"
	"io"
	"net/netip"
)

type (
	Tracker interface {
		Announce(ctx context.Context, t *torrent.Torrent, data *AnnounceData) ([]netip.AddrPort, error)
		Url() string
		WaitInterval(ctx context.Context) error
		io.Closer
	}

	AnnounceData struct {
		Downloaded uint64
		Uploaded   uint64
		Left       uint64
		Port       int
	}
)
