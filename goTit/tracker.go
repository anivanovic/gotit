package main

import (
	"net/url"
	"time"

	"io"

	log "github.com/sirupsen/logrus"
)

const (
	timeout            = time.Second * 1
	protocol_id uint64 = 0x41727101980
)

type Tracker interface {
	Announce(torrent *Torrent) (*map[string]bool, error)
	io.Closer
}

func CreateTracker(urlString string) (Tracker, error) {
	url, err := url.Parse(urlString)
	if err != nil {
		CheckError(err)
		return nil, err
	}

	switch url.Scheme {
	case "udp":
		return udpTracker(url)
	case "http":
		return httpTracker(url), nil
	default:
		log.WithField("url", urlString).Warn("Unsupported tracker protocol")
		return nil, nil // return error
	}
}
