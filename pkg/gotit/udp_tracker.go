package gotit

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"

	"bytes"

	"errors"

	"github.com/anivanovic/gotit/pkg/bencode"
	"go.uber.org/zap"
)

const (
	connect = iota
	announce
	scrape
	con_error
)

const (
	none = iota
	completed
	started
	stopped
)

// BEP15
type udp_tracker struct {
	Conn             *net.UDPConn
	AnnounceInterval int
	Ips              map[string]struct{}

	connectionId  uint64
	transactionId uint32
}

func udpTracker(url *url.URL) (Tracker, error) {
	raddr, err := net.ResolveUDPAddr(url.Scheme, url.Host)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP(url.Scheme, nil, raddr)
	if err != nil {
		return nil, err
	}

	tracker := udp_tracker{conn, 0, nil, 0, 0}
	return &tracker, nil
}

func (t *udp_tracker) Close() error {
	return t.Conn.Close()
}

func (t *udp_tracker) Announce(ctx context.Context, mng *torrentManager) (map[string]struct{}, error) {
	connId, err := t.handshake(ctx)
	if err != nil {
		return nil, err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(timeout)
	}
	t.Conn.SetDeadline(deadline)

	transactionId := createTransactionId()
	request := createAnnounce(connId, transactionId, mng)
	t.Conn.Write(request)
	log.Info("Announce sent to tracker", zap.Stringer("ip", t.Conn.RemoteAddr()))
	response, err := readConn(context.TODO(), t.Conn)
	if err != nil {
		return nil, err
	}

	err = t.readTrackerResponse(response, transactionId)
	return t.Ips, err
}

func (t *udp_tracker) handshake(ctx context.Context) (uint64, error) {
	transactionId := createTransactionId()
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, protocol_id)
	binary.Write(request, binary.BigEndian, uint32(connect))
	binary.Write(request, binary.BigEndian, transactionId)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(timeout)
	}
	t.Conn.SetDeadline(deadline)
	_, err := t.Conn.Write(request.Bytes())
	if err != nil {
		return 0, err
	}
	log.Info("Sent handshake to tracker")

	response := make([]byte, 16)
	_, _, err = t.Conn.ReadFromUDP(response)
	if err != nil {
		return 0, err
	}
	t.readTrackerResponse(response, transactionId)

	return t.connectionId, nil
}

func (t *udp_tracker) readTrackerResponse(response []byte, transactionId uint32) error {
	actionCode := int(binary.BigEndian.Uint32(response[:4]))
	var err error

	switch actionCode {
	case connect:
		t.connectionId, err = readConnect(response, transactionId)
		return err
	case announce:
		t.Ips, err = readAnnounce(response, transactionId)
		return err
	case scrape:
		return readScrape(response, transactionId)
	case con_error:
		// reads error message and always returns error
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

func readAnnounce(response []byte, transactionId uint32) (map[string]struct{}, error) {
	if len(response) < 20 {
		return nil, errors.New("udp announce respons size less then 20")
	}
	if err := checkResponseTransactionId(response, transactionId); err != nil {
		return nil, err
	}

	interval := binary.BigEndian.Uint32(response[8:12])
	leachers := binary.BigEndian.Uint32(response[12:16])
	seaders := binary.BigEndian.Uint32(response[16:20])

	log.Info("CreateTracker message",
		zap.Int("resCode", announce),
		zap.Uint32("interval", interval),
		zap.Uint32("leachers", leachers),
		zap.Uint32("seaders", seaders))
	return parseCompactPeers(bencode.StringElement(response[20:])), nil
}

func readScrape(response []byte, transactionId uint32) error {
	// TODO implement srcape request
	return nil
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
