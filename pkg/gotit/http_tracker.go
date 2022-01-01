package gotit

import (
	"net/http"
	"net/url"

	"strconv"

	"io/ioutil"

	"errors"

	"encoding/binary"
	"net"

	"github.com/anivanovic/gotit/pkg/bencode"
	log "github.com/sirupsen/logrus"
)

type http_tracker struct {
	Url              *url.URL
	AnnounceInterval int
	Ips              *map[string]bool
}

func httpTracker(url *url.URL) Tracker {
	t := new(http_tracker)
	t.Url = url

	return t
}

func (t *http_tracker) Announce(torrent *Torrent) (map[string]struct{}, error) {
	query := t.Url.Query()
	query.Set("info_hash", string(torrent.Hash))
	query.Set("peer_id", string(torrent.PeerId))
	query.Set("port", strconv.Itoa(int(9505))) // TODO here goes listen port
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", "0")
	query.Set("compact", "1")
	t.Url.RawQuery = query.Encode()

	res, err := http.Get(t.Url.String())

	if err != nil {
		CheckError(err)
		return nil, err
	}

	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	CheckError(err)

	body := string(data)
	logger := log.WithFields(log.Fields{
		"statusCode": res.StatusCode,
		"body":       body,
		"url":        t.Url.String(),
	})
	if res.StatusCode != 200 {
		logger.Warn("Invalid request to http tracker")
		return nil, errors.New("http tracker returned response code " + strconv.Itoa(res.StatusCode))
	}

	logger.WithField("body", body).Debug("Http tracker announce response")
	dict, _ := bencode.Parse(body)
	ips := readHttpAnnounce(dict)
	return ips, nil
}

func (t *http_tracker) Close() error { return nil }

func readHttpAnnounce(elem bencode.Bencode) map[string]struct{} {
	if benDict, ok := elem.(bencode.DictElement); ok {
		peers := benDict.Value("peers").String()
		ipData := []byte(peers)
		size := len(ipData)
		peerCount := size / 6
		ips := make(map[string]struct{})
		for read := 0; read < peerCount; read++ {
			byteMask := 6

			ipAddress := net.IPv4(ipData[byteMask*read], ipData[byteMask*read+1], ipData[byteMask*read+2], ipData[byteMask*read+3])
			port := binary.BigEndian.Uint16(ipData[byteMask*read+4 : byteMask*read+6])
			ipAddr := ipAddress.String() + ":" + strconv.Itoa(int(port))
			ips[ipAddr] = struct{}{}
		}
		return ips
	}
	return nil
}
