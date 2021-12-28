package gotit

import (
	"net"
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

func readConn(conn net.Conn) []byte {
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	for {
		conn.SetDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		log.WithField("read data", n).Info("Read data from connection")
		response = append(response, tmp[:n]...)
	}

	return response
}

func CheckError(err error) {
	if err != nil {
		log.Warnf("%T %+v", err, err)
	}
}
