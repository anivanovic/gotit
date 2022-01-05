package gotit

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/url"

	"strconv"

	"io/ioutil"

	"errors"

	"github.com/anivanovic/gotit/pkg/bencode"
	log "github.com/sirupsen/logrus"
)

type http_tracker struct {
	Url *url.URL
}

func httpTracker(url *url.URL) Tracker {
	t := new(http_tracker)
	t.Url = url

	return t
}

func (t http_tracker) Announce(ctx context.Context, torrent *Torrent) (map[string]struct{}, error) {
	query := t.Url.Query()
	query.Set("info_hash", string(torrent.Hash))
	query.Set("peer_id", string(torrent.PeerId))
	query.Set("port", strconv.Itoa(int(9505))) // TODO here goes listen port
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", "0")
	query.Set("compact", "1")
	t.Url.RawQuery = query.Encode()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, t.Url.String(), nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

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

	logger.Debug("Http tracker announce response")
	benc, err := bencode.Parse(body)
	if err != nil {
		return nil, err
	}
	return readPeers(benc[0])
}

func (t *http_tracker) Close() error { return nil }

func readPeers(elem bencode.Bencode) (map[string]struct{}, error) {
	if benDict, ok := elem.(bencode.DictElement); ok {
		peers := benDict.Value("peers")
		if peers == nil {
			return nil, errors.New("tracker: no peers in announce response")
		}

		switch peers := peers.(type) {
		case bencode.ListElement:
			return parseListPeers(peers), nil
		case bencode.StringElement:
			return parseStringPeers(peers), nil
		default:
			return nil, errors.New("tracker: announce peers of wrong type")
		}
	}

	return nil, errors.New("tracker: no peers in announce response")
}

func parseListPeers(peers bencode.ListElement) map[string]struct{} {
	ips := make(map[string]struct{})
	for _, p := range peers {
		data, ok := p.(bencode.DictElement)
		if !ok {
			continue
		}

		ip := data.Value("ip").String()
		peerId := data.Value("peer id")
		port := data.Value("port").String()
		ipAddr := ip + ":" + port
		log.Debugf("ip: %s, port: %s, peerId: %s", ip, port, peerId)
		ips[ipAddr] = struct{}{}
	}
	return ips
}

func parseStringPeers(peers bencode.StringElement) map[string]struct{} {
	ipData := []byte(peers.String())
	peerCount := len(ipData) / 6

	byteMask, ips := 6, make(map[string]struct{})
	for read := 0; read < peerCount; read++ {
		ipAddress := net.IPv4(
			ipData[byteMask*read],
			ipData[byteMask*read+1],
			ipData[byteMask*read+2],
			ipData[byteMask*read+3])
		port := binary.BigEndian.Uint16(ipData[byteMask*read+4 : byteMask*read+6])
		ipAddr := ipAddress.String() + ":" + strconv.Itoa(int(port))
		ips[ipAddr] = struct{}{}
	}
	return ips
}
