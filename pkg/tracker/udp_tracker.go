package tracker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"net/url"
	"time"

	"github.com/anivanovic/gotit"
	"github.com/anivanovic/gotit/pkg/peer"

	"github.com/anivanovic/gotit/pkg/gotitnet"
	"go.uber.org/zap"
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

	log *zap.Logger
	waitInterval
}

func newUdpTracker(url *url.URL, logger *zap.Logger) (*udpTracker, error) {
	conn, err := gotitnet.NewTimeoutConn(url.Scheme, url.Host, gotitnet.TrackerTimeout)
	if err != nil {
		return nil, err
	}

	tracker := udpTracker{
		conn:         conn,
		waitInterval: waitInterval{time.Minute},
		url:          url.String(),
		log:          logger,
	}
	return &tracker, nil
}

func (t *udpTracker) Url() string {
	return t.url
}

func (t *udpTracker) Close() error {
	return t.conn.Close()
}

func (t *udpTracker) Announce(ctx context.Context, torrentHash string, data *gotit.AnnounceData) ([]netip.AddrPort, error) {
	connId, err := t.handshake(ctx)
	if err != nil {
		return nil, err
	}

	transactionId := createTransactionId()
	// TODO propagate download stats
	request := createAnnounce(connId, transactionId, torrentHash, data)
	t.conn.Write(request)
	t.log.Info("Announce sent to tracker", zap.String("ip", t.Url()))
	response, err := t.conn.ReadAll()
	if err != nil {
		return nil, err
	}

	err = t.parseTrackerResponse(response, transactionId)
	return t.ips, err
}

func (t *udpTracker) handshake(ctx context.Context) (uint64, error) {
	transactionId := createTransactionId()
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, protocolId)
	binary.Write(request, binary.BigEndian, uint32(connect))
	binary.Write(request, binary.BigEndian, transactionId)

	_, err := t.conn.Write(request.Bytes())
	if err != nil {
		return 0, err
	}
	t.log.Info("Sent handshake to tracker")

	response, err := t.conn.ReadUdpHandshake()
	if err != nil {
		return 0, err
	}
	if err := t.parseTrackerResponse(response, transactionId); err != nil {
		return 0, err
	}

	return t.connectionId, nil
}

func (t *udpTracker) parseTrackerResponse(response []byte, transactionId uint32) error {
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

func createAnnounce(connId uint64, transactionId uint32, torrentHash string, data *gotit.AnnounceData) []byte {
	request := &bytes.Buffer{}
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(announce))
	binary.Write(request, binary.BigEndian, transactionId)
	binary.Write(request, binary.BigEndian, torrentHash)
	binary.Write(request, binary.BigEndian, peer.ClientId)
	binary.Write(request, binary.BigEndian, data.Downloaded)
	binary.Write(request, binary.BigEndian, data.Left)
	binary.Write(request, binary.BigEndian, data.Uploaded)
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
	return conId, nil
}

func (t *udpTracker) readAnnounce(response []byte, transactionId uint32) ([]netip.AddrPort, error) {
	if len(response) < 20 {
		return nil, errors.New("udp tracker invalid announce response size")
	}
	if err := checkResponseTransactionId(response, transactionId); err != nil {
		return nil, err
	}

	t.interval = time.Duration(binary.BigEndian.Uint32(response[8:12])) * time.Second
	leechers := binary.BigEndian.Uint32(response[12:16])
	seeders := binary.BigEndian.Uint32(response[16:20])

	t.log.Info("CreateTracker message",
		zap.Int("resCode", announce),
		zap.Duration("interval", t.interval),
		zap.Uint32("leechers", leechers),
		zap.Uint32("seeders", seeders))
	return parseCompactPeers(response[20:]), nil
}

func readError(response []byte, transactionId uint32) error {
	if len(response) < 8 {
		return errors.New("udp error respons size less then 8")
	}
	if err := checkResponseTransactionId(response, transactionId); err != nil {
		return err
	}

	message := string(response[8:])
	return errors.New("udp error response: " + message)
}

func createTransactionId() uint32 {
	return rand.Uint32()
}

func checkResponseTransactionId(response []byte, transactionId uint32) error {
	responseId := binary.BigEndian.Uint32(response[4:8])
	if transactionId != responseId {
		return errors.New("udp tracker response transaction_id not the same as sent")
	}

	return nil
}
