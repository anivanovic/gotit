package gotit

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"io"
)

const (
	timeout            = time.Second * 1
	protocol_id uint64 = 0x41727101980
)

type Tracker interface {
	Announce(ctx context.Context, t *torrentManager) (map[string]struct{}, error)
	io.Closer
}

func CreateTracker(urlString string) (Tracker, error) {
	url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}

	switch url.Scheme {
	case "udp":
		return udpTracker(url)
	case "http", "https":
		return httpTracker(url), nil
	default:
		return nil, fmt.Errorf("tracker: unsupported protocol %s", url.Scheme)
	}
}
