package tracker

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"time"

	"github.com/anivanovic/gotit/pkg/torrent"

	"io"
)

type Tracker interface {
	Announce(ctx context.Context, t *torrent.Torrent, data *AnnounceData) ([]netip.AddrPort, error)
	Url() string
	WaitInterval(ctx context.Context) error
	io.Closer
}

type AnnounceData struct {
	Downloaded uint64
	Uploaded   uint64
	Left       uint64

	Port int
}

type waitInterval struct {
	interval time.Duration
}

func (t waitInterval) WaitInterval(ctx context.Context) error {
	select {
	case <-time.After(t.interval):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func NewTracker(urlString string) (Tracker, error) {
	url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}

	switch url.Scheme {
	case "udp":
		return newUdpTracker(url)
	case "http", "https":
		return newHttpTracker(url), nil
	default:
		return nil, fmt.Errorf("tracker: unsupported protocol %s", url.Scheme)
	}
}
