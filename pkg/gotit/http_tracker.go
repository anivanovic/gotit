package gotit

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"strconv"

	"io/ioutil"

	"errors"

	"github.com/anivanovic/gotit/pkg/bencode"
	"go.uber.org/zap"
)

type http_tracker struct {
	Url          *url.URL
	Interval     time.Duration
	lastAnnounce time.Time
}

func httpTracker(url *url.URL) Tracker {
	t := new(http_tracker)
	t.Url = url

	return t
}

func (t *http_tracker) Announce(ctx context.Context, torrent *Torrent) (map[string]struct{}, error) {
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

	t.lastAnnounce = time.Now()
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
	if res.StatusCode != 200 {
		log.Warn("Tracker response with error status code",
			zap.Int(
				"statusCode", res.StatusCode),
			zap.String("body", body),
			zap.Stringer("url", t.Url))
		return nil, errors.New("http tracker error response code " + strconv.Itoa(res.StatusCode))
	}

	benc, err := bencode.Parse(body)
	if err != nil {
		return nil, err
	}
	dict := benc[0].(bencode.DictElement)
	failure := dict.Value("failure reason")
	if failure != nil {
		return nil, fmt.Errorf("tracker returned failure reason: %s", failure)
	}
	if interval := dict.Value("interval"); interval != nil {
		t.Interval = time.Second * time.Duration(interval.(bencode.IntElement))
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
			return parseBencodePeers(peers), nil
		case bencode.StringElement:
			return parseCompactPeers(peers), nil
		default:
			return nil, errors.New("tracker: peers of wrong type")
		}
	}

	return nil, errors.New("tracker: no peers in announce response")
}

func parseBencodePeers(peers bencode.ListElement) map[string]struct{} {
	ips := make(map[string]struct{})
	for _, p := range peers {
		data, ok := p.(bencode.DictElement)
		if !ok {
			continue
		}

		ip := data.Value("ip").String()
		// TODO: do we need peerId
		// peerId := data.Value("peer id")
		port := data.Value("port").String()
		ipAddr := ip + ":" + port
		ips[ipAddr] = struct{}{}
	}
	return ips
}

func parseCompactPeers(peers bencode.StringElement) map[string]struct{} {
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
