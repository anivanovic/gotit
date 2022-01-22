package gotit

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net/url"
	"time"

	"bytes"

	"errors"

	"github.com/anivanovic/gotit/pkg/bencode"
	"go.uber.org/zap"
)

const protocolId uint64 = 0x41727101980

const (
	connect = iota
	announce
	con_error
)

const (
	none = iota
	completed
	started
	stopped
)

// BEP15
type udpTracker struct {
	conn *timeoutConn
	ips  []string

	connectionId  uint64
	transactionId uint32

	waitInterval
}

func newUdpTracker(url *url.URL) (Tracker, error) {
	conn, err := NewTimeoutConn(url.Scheme, url.Host, trackerTimeout)
	if err != nil {
		return nil, err
	}

	tracker := udpTracker{conn, nil, 0, 0, waitInterval{time.Minute}}
	return &tracker, nil
}

func (t udpTracker) Url() string {
	return t.conn.c.RemoteAddr().String()
}

func (t *udpTracker) Close() error {
	return t.conn.c.Close()
}

func (t *udpTracker) Announce(ctx context.Context, mng *torrentManager) ([]string, error) {
	connId, err := t.handshake(ctx)
	if err != nil {
		return nil, err
	}

	transactionId := createTransactionId()
	request := createAnnounce(connId, transactionId, mng)
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
	case con_error:
		return readError(response, transactionId)
	default:
		return fmt.Errorf("unrecognized udp tracker response action code: %d", actionCode)
	}
}

func createAnnounce(connId uint64, transactionId uint32, mng *torrentManager) []byte {
	request := &bytes.Buffer{}
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(announce))
	binary.Write(request, binary.BigEndian, transactionId)
	binary.Write(request, binary.BigEndian, mng.torrent.Hash)
	binary.Write(request, binary.BigEndian, mng.torrent.PeerId)
	binary.Write(request, binary.BigEndian, uint64(mng.torrentStatus.Download()))
	binary.Write(request, binary.BigEndian, uint64(mng.torrentStatus.Left()))
	binary.Write(request, binary.BigEndian, uint64(mng.torrentStatus.Upload()))
	binary.Write(request, binary.BigEndian, uint32(none))
	binary.Write(request, binary.BigEndian, uint32(0))
	binary.Write(request, binary.BigEndian, rand.Int31())
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, int32(mng.listenPort))
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

func (t *udpTracker) readAnnounce(response []byte, transactionId uint32) ([]string, error) {
	if len(response) < 20 {
		return nil, errors.New("udp announce respons size less then 20")
	}
	if err := checkResponseTransactionId(response, transactionId); err != nil {
		return nil, err
	}

	t.interval = time.Duration(binary.BigEndian.Uint32(response[8:12])) * time.Second
	leachers := binary.BigEndian.Uint32(response[12:16])
	seaders := binary.BigEndian.Uint32(response[16:20])

	log.Info("CreateTracker message",
		zap.Int("resCode", announce),
		zap.Duration("interval", t.interval),
		zap.Uint32("leachers", leachers),
		zap.Uint32("seaders", seaders))
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
