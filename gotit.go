package gotit

import (
	"context"
	"io"
	"net/netip"
)

type (
	Tracker interface {
		Announce(ctx context.Context, torrentHash string, data *AnnounceData) ([]netip.AddrPort, error)
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

	AnnounceResponse struct {
		Failure      string         `ben:"failure reason,optional"`
		Interval     int            `ben:"interval,optional"`
		Peers        []AnnouncePeer `ben:"peers,optional"`
		PeersCompact []byte         `ben:"peers,optional"`
		PeersIpv6    []byte         `ben:"peers6,optional"`
	}

	AnnouncePeer struct {
		//Id   string `ben:"id"`
		Ip   string `ben:"ip"`
		Port string `ben:"port"`
	}
)
