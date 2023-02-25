package tracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/netip"
	"net/url"
	"time"

	"github.com/anivanovic/gotit/pkg/gotitnet"
	"github.com/anivanovic/gotit/pkg/torrent"

	"bytes"

	"errors"

	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
)

const protocolId uint64 = 0x41727101980

const (
	connect = iota
	announce
	conError
)

const (
	none = iota
	completed
	started
	stopped
)

// BEP15
type udpTracker struct {
	conn *gotitnet.TimeoutConn
	ips  []netip.AddrPort
	url  string

	connectionId  uint64
	transactionId uint32

	waitInterval
}

func newUdpTracker(url *url.URL) (Tracker, error) {
	conn, err := gotitnet.NewTimeoutConn(url.Scheme, url.Host, gotitnet.TrackerTimeout)
	if err != nil {
		return nil, err
	}

	tracker := udpTracker{
		conn:         conn,
		waitInterval: waitInterval{time.Minute},
		url:          url.String(),
	}
	return &tracker, nil
}

func (t udpTracker) Url() string {
	return t.url
}

func (t *udpTracker) Close() error {
	return t.conn.Close()
}

func (t *udpTracker) Announce(ctx context.Context, torrent *torrent.Torrent, data *AnnounceData) ([]netip.AddrPort, error) {
	connId, err := t.handshake(ctx)
	if err != nil {
		return nil, err
	}

	transactionId := createTransactionId()
	// TODO propagate download stats
	request := createAnnounce(connId, transactionId, torrent, data)
	t.conn.Write(ctx, request)
	log.Info("Announce sent to tracker", zap.String("ip", t.Url()))
	response, err := t.conn.ReadAll(context.TODO())
	if err != nil {
		return nil, err
	}

	err = t.readTrackerResponse(response, transactionId)
	return t.ips, err
}

func (t *udpTracker) handshake(ctx context.Context) (uint64, error) {
	transactionId := createTransactionId()
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, protocolId)
	binary.Write(request, binary.BigEndian, uint32(connect))
	binary.Write(request, binary.BigEndian, transactionId)

	_, err := t.conn.Write(ctx, request.Bytes())
	if err != nil {
		return 0, err
	}
	log.Info("Sent handshake to tracker")

	response, err := t.conn.ReadUdpHandshake(ctx)
	if err != nil {
		return 0, err
	}
	t.readTrackerResponse(response, transactionId)

	return t.connectionId, nil
}

func (t *udpTracker) readTrackerResponse(response []byte, transactionId uint32) error {
	actionCode := int(binary.BigEndian.Uint32(response[:4]))
	var err error

	switch actionCode {
	case connect:
		t.connectionId, err = readConnect(response, transactionId)
		return err
	case announce:
		t.ips, err = t.readAnnounce(response, transactionId)
		return err
	case conError:
		return readError(response, transactionId)
	default:
		return fmt.Errorf("unrecognized udp tracker response action code: %d", actionCode)
	}
}

func createAnnounce(connId uint64, transactionId uint32, torrent *torrent.Torrent, data *AnnounceData) []byte {
	request := &bytes.Buffer{}
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(announce))
	binary.Write(request, binary.BigEndian, transactionId)
	binary.Write(request, binary.BigEndian, torrent.Hash)
	binary.Write(request, binary.BigEndian, torrent.PeerId)
	binary.Write(request, binary.BigEndian, uint64(data.Downloaded))
	binary.Write(request, binary.BigEndian, uint64(data.Left))
	binary.Write(request, binary.BigEndian, uint64(data.Uploaded))
	binary.Write(request, binary.BigEndian, uint32(none))
	binary.Write(request, binary.BigEndian, uint32(0))
	binary.Write(request, binary.BigEndian, rand.Int31())
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, int32(data.Port))
	return request.Bytes()
}

func readConnect(data []byte, transactionId uint32) (uint64, error) {
	if len(data) < 16 {
		return 0, errors.New("udp connect respons size less then 16")
	}
	if err := checkResponseTransactionId(data, transactionId); err != nil {
		return 0, err
	}

	conId := binary.BigEndian.Uint64(data[8:16])
	log.Info("CreateTracker handshake response",
		zap.Uint64("conn id", conId),
		zap.Int("res code", connect))

	return conId, nil
}

func (t *udpTracker) readAnnounce(response []byte, transactionId uint32) ([]netip.AddrPort, error) {
	if len(response) < 20 {
		return nil, errors.New("udp announce respons size less then 20")
	}
	if err := checkResponseTransactionId(response, transactionId); err != nil {
		return nil, err
	}

	t.interval = time.Duration(binary.BigEndian.Uint32(response[8:12])) * time.Second
	leechers := binary.BigEndian.Uint32(response[12:16])
	seeders := binary.BigEndian.Uint32(response[16:20])

	log.Info("CreateTracker message",
		zap.Int("resCode", announce),
		zap.Duration("interval", t.interval),
		zap.Uint32("leechers", leechers),
		zap.Uint32("seeders", seeders))
	return parseCompactPeers(bencode.StringElement(response[20:])), nil
}

func readError(response []byte, transactionId uint32) error {
	if len(response) < 8 {
		return errors.New("udp error respons size less then 8")
	}
	if err := checkResponseTransactionId(response, transactionId); err != nil {
		return err
	}

	message := string(response[8:])
	return errors.New("udp error respons: " + message)
}

func createTransactionId() uint32 {
	return rand.Uint32()
}

func checkResponseTransactionId(response []byte, transactionId uint32) error {
	responseId := binary.BigEndian.Uint32(response[4:8])
	if transactionId != responseId {
		return errors.New("udp response transaction_id not the same as sent")
	}

	return nil
}
