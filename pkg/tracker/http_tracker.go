package tracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/anivanovic/gotit/pkg/torrent"

	"strconv"

	"io/ioutil"

	"errors"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
)

type tEvent string

const (
	startedEvent   tEvent = "started"
	stoppedEvent   tEvent = "stopped"
	completedEvent tEvent = "completed"
)

var log = zap.L()

type httpTracker struct {
	url       *url.URL
	trackerId string
	event     tEvent

	waitInterval
}

func newHttpTracker(url *url.URL) Tracker {
	t := &httpTracker{
		url:       url,
		event:     startedEvent,
		trackerId: uuid.NewString(),
		waitInterval: waitInterval{
			interval: time.Minute,
		},
	}

	return t
}

func (t httpTracker) Url() string {
	return t.url.String()
}

func (t *httpTracker) Close() error { return nil }

func (t *httpTracker) Announce(ctx context.Context, torrent *torrent.Torrent, data *AnnounceData) ([]netip.AddrPort, error) {
	t.buildQuer(torrent, data)
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, t.Url(), nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		log.Warn("Tracker response with error status code",
			zap.Int(
				"statusCode", res.StatusCode),
			zap.String("body", string(body)),
			zap.String("url", t.Url()))
		return nil, errors.New("http tracker error response code " + strconv.Itoa(res.StatusCode))
	}

	return t.readPeers(body)
}

func (t *httpTracker) buildQuer(torrent *torrent.Torrent, data *AnnounceData) {
	query := t.url.Query()
	query.Set("info_hash", string(torrent.Hash))
	query.Set("peer_id", string(torrent.PeerId))
	query.Set("port", strconv.Itoa(data.Port))
	query.Set("downloaded", strconv.FormatUint(data.Downloaded, 10))
	query.Set("uploaded", strconv.FormatUint(data.Uploaded, 10))
	query.Set("left", strconv.FormatUint(data.Left, 10))
	query.Set("numwant", "50")
	query.Set("event", string(t.event))
	query.Set("trackerid", t.trackerId)
	query.Set("no_peer_id", "1")
	query.Set("compact", "1")
	t.url.RawQuery = query.Encode()
}

func (t *httpTracker) readPeers(res []byte) ([]netip.AddrPort, error) {
	benc, err := bencode.Parse(res)
	if err != nil {
		return nil, err
	}

	if dict, ok := benc.(*bencode.DictElement); ok {
		failure := dict.Value("failure reason")
		if failure != nil {
			return nil, fmt.Errorf("tracker returned failure reason: %s", failure)
		}
		if interval := dict.Value("interval"); interval != nil {
			t.interval = time.Duration(interval.(bencode.IntElement)) * time.Second
		}

		peers := dict.Value("peers")
		if peers == nil {
			return nil, errors.New("tracker: no peers in announce response")
		}

		switch peers := peers.(type) {
		case *bencode.ListElement:
			return parseBencodePeers(peers), nil
		case bencode.StringElement:
			return parseCompactPeers(peers), nil
		default:
			return nil, fmt.Errorf("tracker: expected List or String bencode peers element, got: %T", peers)
		}
	}

	return nil, errors.New("tracker: response not bencoded dictionary")
}

func parseBencodePeers(peers *bencode.ListElement) []netip.AddrPort {
	ips := make([]netip.AddrPort, len(peers.Value))
	for _, p := range peers.Value {
		data, ok := p.(*bencode.DictElement)
		if !ok {
			continue
		}

		ip := data.Value("ip").String()
		port := data.Value("port").String()
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			log.Error("invalid ip address", zap.Error(err), zap.String("ip", ip))
			continue
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil {
			log.Error("port not number", zap.String("port", port))
			continue
		}
		parseIP := addr
		addrPort := netip.AddrPortFrom(parseIP, uint16(portNumber))
		ips = append(ips, addrPort)
	}
	return ips
}

func parseCompactPeers(peers bencode.StringElement) []netip.AddrPort {
	ipData := []byte(peers)
	peerCount := len(ipData) / 6

	byteMask, ips := 6, make([]netip.AddrPort, peerCount)
	for read := 0; read < peerCount; read++ {
		var ip [4]byte
		ip[0] = ipData[(byteMask * read)]
		ip[1] = ipData[(byteMask*read + 1)]
		ip[2] = ipData[(byteMask*read + 2)]
		ip[3] = ipData[(byteMask*read + 3)]
		addr := netip.AddrFrom4(ip)
		port := binary.BigEndian.Uint16(ipData[byteMask*read+4 : byteMask*read+6])
		addrPort := netip.AddrPortFrom(addr, port)
		ips = append(ips, addrPort)
	}
	return ips
}