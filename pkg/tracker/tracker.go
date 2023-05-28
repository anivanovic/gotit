package tracker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/anivanovic/gotit"
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

func New(addr string) (gotit.Tracker, error) {
	parsedAddr, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	switch parsedAddr.Scheme {
	case "udp":
		return newUdpTracker(parsedAddr)
	case "http", "https":
		return newHttpTracker(addr, http.DefaultClient), nil
	default:
		return nil, fmt.Errorf("unsupported tracker protocol %s", parsedAddr.Scheme)
	}
}
