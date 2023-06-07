package tracker

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"github.com/anivanovic/gotit"

	"github.com/anivanovic/gotit/pkg/torrent"

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

type httpTracker struct {
	c         *http.Client
	addr      string
	trackerId string
	event     tEvent

	logger *zap.Logger
	waitInterval
}

func newHttpTracker(addr string, client *http.Client, logger *zap.Logger) *httpTracker {
	t := &httpTracker{
		c:         client,
		logger:    logger,
		addr:      addr,
		event:     startedEvent,
		trackerId: uuid.NewString(),
		waitInterval: waitInterval{
			interval: time.Minute,
		},
	}

	return t
}

func (t *httpTracker) Url() string {
	return t.addr
}

func (t *httpTracker) Close() error { return nil }

func (t *httpTracker) Announce(ctx context.Context, torrent *torrent.Torrent, data *gotit.AnnounceData) ([]netip.AddrPort, error) {
	q := t.buildQuery(torrent, data)
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?%s", t.addr, q.Encode()), nil)
	if err != nil {
		return nil, err
	}

	res, err := t.c.Do(r)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.logger.Warn("error closing announce response",
				zap.Error(err),
				zap.String("tracker addr", t.addr))
		}
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		t.logger.Warn("Tracker response with error status code",
			zap.Int("statusCode", res.StatusCode),
			zap.ByteString("body", body),
			zap.String("addr", t.Url()))
		return nil, fmt.Errorf("tracker error response [%d]: %s", res.StatusCode, body)
	}

	return t.readPeers(body)
}

func (t *httpTracker) buildQuery(torrent *torrent.Torrent, data *gotit.AnnounceData) *url.Values {
	query := url.Values{}
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
	return &query
}

func (t *httpTracker) readPeers(res []byte) ([]netip.AddrPort, error) {
	var announceResponse gotit.AnnounceResponse
	if err := bencode.Unmarshal(res, &announceResponse); err != nil {
		return nil, err
	}
	if announceResponse.Failure != "" {
		return nil, fmt.Errorf("tracker failure: %s", announceResponse.Failure)
	}
	t.interval = time.Duration(announceResponse.Interval) * time.Second

	if announceResponse.Peers != nil {
		return t.parseBencodePeers(announceResponse.Peers), nil
	}
	if announceResponse.PeersCompact != nil {
		return parseCompactPeers(announceResponse.PeersCompact), nil
	}
	if announceResponse.PeersIpv6 != nil {
		// TODO: parse ipv6 compact response
	}

	return nil, errors.New("successful tracker response without ")
}

func (t *httpTracker) parseBencodePeers(peers []gotit.AnnouncePeer) []netip.AddrPort {
	ips := make([]netip.AddrPort, len(peers))
	for _, p := range peers {
		ip, err := netip.ParseAddr(p.Ip)
		if err != nil {
			t.logger.Error(
				"tracker sent invalid peer ip address",
				zap.Error(err),
				zap.String("ip", p.Ip),
				zap.String("port", p.Port),
			)
			continue
		}
		port, err := strconv.ParseUint(p.Port, 10, 16)
		if err != nil {
			t.logger.Error(
				"parsing port",
				zap.Error(err),
			)
			continue
		}
		ips = append(ips, netip.AddrPortFrom(ip, uint16(port)))
	}

	return ips
}

func parseCompactPeers(peers []byte) []netip.AddrPort {
	peerCount := len(peers) / 6

	byteMask, ips := 6, make([]netip.AddrPort, peerCount)
	for read := 0; read < peerCount; read++ {
		var ip [4]byte
		ip[0] = peers[(byteMask * read)]
		ip[1] = peers[(byteMask*read + 1)]
		ip[2] = peers[(byteMask*read + 2)]
		ip[3] = peers[(byteMask*read + 3)]
		addr := netip.AddrFrom4(ip)
		port := binary.BigEndian.Uint16(peers[byteMask*read+4 : byteMask*read+6])
		addrPort := netip.AddrPortFrom(addr, port)
		ips = append(ips, addrPort)
	}
	return ips
}
