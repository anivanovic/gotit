package tracker

import (
	"context"
	"fmt"
	"github.com/anivanovic/gotit"
	"net/url"
	"time"
)

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

func NewTracker(urlString string) (gotit.Tracker, error) {
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
