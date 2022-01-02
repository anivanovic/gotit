package gotit

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"time"

	"bytes"

	"errors"

	log "github.com/sirupsen/logrus"
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
		CheckError(err)
		return nil, err
	}

	conn, err := net.DialUDP(url.Scheme, nil, raddr)
	if err != nil {
		CheckError(err)
		return nil, err
	}

	tracker := udp_tracker{conn, 0, nil, 0, 0}
	return &tracker, nil
}

func (t *udp_tracker) Close() error {
	return t.Conn.Close()
}

func (t *udp_tracker) Announce(ctx context.Context, torrent *Torrent) (map[string]struct{}, error) {
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
	request := createAnnounce(connId, transactionId, torrent)
	t.Conn.Write(request)
	log.WithField("ip", t.Conn.RemoteAddr()).Info("Announce sent to tracker")
	response := readConn(t.Conn)

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
	case announce:
		t.Ips, err = readAnnounce(response, transactionId)
	case scrape:
		err = readScrape(response, transactionId)
	case con_error:
		// reads error message and always returns error
		return readError(response, transactionId)
	default:
		err = fmt.Errorf("unrecognized udp tracker response action code: %d", actionCode)
	}

	return err
}

func createAnnounce(connId uint64, transactionId uint32, torrent *Torrent) []byte {
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(announce))
	binary.Write(request, binary.BigEndian, transactionId)
	binary.Write(request, binary.BigEndian, torrent.Hash)
	binary.Write(request, binary.BigEndian, torrent.PeerId)
	binary.Write(request, binary.BigEndian, uint64(torrent.downloaded))
	binary.Write(request, binary.BigEndian, uint64(torrent.left))
	binary.Write(request, binary.BigEndian, uint64(torrent.uploaded))
	binary.Write(request, binary.BigEndian, uint32(none))
	binary.Write(request, binary.BigEndian, uint32(0))
	randKey := rand.Int31()
	binary.Write(request, binary.BigEndian, randKey)
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, 9404) // TODO here goes listen port
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
	log.WithFields(log.Fields{
		"connection id": conId,
		"resCode":       connect,
	}).Info("CreateTracker handshake response")

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
	peerAddresses := response[20:]
	peerCount := len(peerAddresses) / 6

	log.WithFields(log.Fields{
		"resCode":    announce,
		"interval":   interval,
		"leachers":   leachers,
		"seaders":    seaders,
		"peer count": peerCount,
	}).Info("CreateTracker message")

	ips := make(map[string]struct{})
	for i := 0; i < peerCount; i++ {
		byteMask := 6
		ipAddress := net.IPv4(peerAddresses[byteMask*i], peerAddresses[byteMask*i+1], peerAddresses[byteMask*i+2], peerAddresses[byteMask*i+3])
		port := binary.BigEndian.Uint16(peerAddresses[byteMask*i+4 : byteMask*i+6])
		ipAddr := ipAddress.String() + ":" + strconv.Itoa(int(port))
		ips[ipAddr] = struct{}{}
	}
	return ips, nil
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
