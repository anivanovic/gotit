package main

import (
	"encoding/binary"
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
	CONNECT = iota
	ANNOUNCE
	SCRAPE
	ERROR
)

const (
	NONE = iota
	COMPLETED
	STARTED
	STOPPED
)

// BEP15
type udp_tracker struct {
	Url              *url.URL
	Conn             *net.UDPConn
	AnnounceInterval int
	Ips              *map[string]bool

	addr          *net.UDPAddr
	connectionId  uint64
	transactionId uint32
}

func udpTracker(url *url.URL) (Tracker, error) {
	addr, err := net.ResolveUDPAddr(url.Scheme, url.Host)
	if err != nil {
		CheckError(err)
		return nil, err
	}

	conn, err := net.ListenUDP(url.Scheme, nil)
	if err != nil {
		CheckError(err)
		return nil, err
	}

	tracker := udp_tracker{url, conn, 0, nil, addr, 0, 0}
	return &tracker, nil
}

func (t *udp_tracker) Announce(torrent *Torrent) (*map[string]bool, error) {
	connId, err := t.handshake()
	if err != nil {
		return nil, err
	}

	transactionId := createTransactionId()
	request := createAnnounce(connId, transactionId, torrent)

	t.Conn.SetDeadline(time.Now().Add(timeout))
	t.Conn.WriteTo(request, t.addr)
	log.WithField("ip", t.Url.String()).Info("Announce sent to tracker")
	response := readConn(t.Conn)

	err = t.readTrackerResponse(response, transactionId)
	return t.Ips, err
}

func (t *udp_tracker) Close() error {
	return t.Conn.Close()
}

func (t *udp_tracker) handshake() (uint64, error) {
	request := new(bytes.Buffer)
	transactionId := createTransactionId()

	binary.Write(request, binary.BigEndian, protocol_id)
	binary.Write(request, binary.BigEndian, uint32(CONNECT))
	binary.Write(request, binary.BigEndian, transactionId)

	t.Conn.SetDeadline(time.Now().Add(timeout))
	_, err := t.Conn.WriteTo(request.Bytes(), t.addr)
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

func createAnnounce(connId uint64, transactionId uint32, torrent *Torrent) []byte {
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(ANNOUNCE))
	binary.Write(request, binary.BigEndian, transactionId)
	binary.Write(request, binary.BigEndian, torrent.Hash)
	binary.Write(request, binary.BigEndian, torrent.PeerId)
	binary.Write(request, binary.BigEndian, uint64(torrent.downloaded))
	binary.Write(request, binary.BigEndian, uint64(torrent.left))
	binary.Write(request, binary.BigEndian, uint64(torrent.uploaded))
	binary.Write(request, binary.BigEndian, uint32(NONE))
	binary.Write(request, binary.BigEndian, uint32(0))
	randKey := rand.Int31()
	binary.Write(request, binary.BigEndian, randKey)
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, listenPort)
	return request.Bytes()
}

func (t *udp_tracker) readTrackerResponse(response []byte, transactionId uint32) error {
	actionCode := int(binary.BigEndian.Uint32(response[:4]))
	var err error
	switch actionCode {
	case CONNECT:
		var connId uint64
		connId, err = readConnect(response, transactionId)
		t.connectionId = connId
	case ANNOUNCE:
		t.Ips, err = readUdpAnnounce(response, transactionId)
	case SCRAPE:
		err = readScrape(response, transactionId)
	case ERROR:
		err = readError(response, transactionId)
	default:
		err = errors.New("unrecognized udp tracker response. action code: " + strconv.Itoa(actionCode))
	}

	return err
}

func readConnect(data []byte, transactionId uint32) (uint64, error) {
	if len(data) < 16 {
		return 0, errors.New("udp connect respons size smaller then minimal size (16)")
	}

	if err := checkTransactionId(data, transactionId); err != nil {
		return 0, err
	}

	connId := binary.BigEndian.Uint64(data[8:16])
	log.WithFields(log.Fields{
		"connection id": connId,
		"resCode":       CONNECT,
	}).Info("CreateTracker handshake response")

	return connId, nil
}

func readUdpAnnounce(response []byte, transactionId uint32) (*map[string]bool, error) {
	if len(response) < 20 {
		return nil, errors.New("udp announce respons size smaller then minimal size (20)")
	}
	if err := checkTransactionId(response, transactionId); err != nil {
		return nil, err
	}

	interval := binary.BigEndian.Uint32(response[8:12])
	leachers := binary.BigEndian.Uint32(response[12:16])
	seaders := binary.BigEndian.Uint32(response[16:20])
	peerCount := (len(response) - 20) / 6
	peerAddresses := response[20:]

	log.WithFields(log.Fields{
		"resCode":    ANNOUNCE,
		"interval":   interval,
		"leachers":   leachers,
		"seaders":    seaders,
		"peer count": peerCount,
	}).Info("CreateTracker message")

	ips := make(map[string]bool, 0)
	for read := 0; read < peerCount; read++ {
		byteMask := 6

		ipAddress := net.IPv4(peerAddresses[byteMask*read], peerAddresses[byteMask*read+1], peerAddresses[byteMask*read+2], peerAddresses[byteMask*read+3])
		port := binary.BigEndian.Uint16(peerAddresses[byteMask*read+4 : byteMask*read+6])
		ipAddr := ipAddress.String() + ":" + strconv.Itoa(int(port))
		ips[ipAddr] = true
	}
	return &ips, nil
}

func readScrape(response []byte, transactionId uint32) error {
	// TODO implement srcape request
	return nil
}

func readError(response []byte, transactionId uint32) error {
	if len(response) < 8 {
		return errors.New("udp error respons size smaller then minimal size (8)")
	}
	if err := checkTransactionId(response, transactionId); err != nil {
		return err
	}

	message := string(response[8:])
	return errors.New("udp error respons: " + message)
}

func createTransactionId() uint32 {
	return rand.Uint32()
}

func checkTransactionId(response []byte, transactionId uint32) error {
	resTransactionId := binary.BigEndian.Uint32(response[4:8])
	if transactionId != resTransactionId {
		return errors.New("udp response transaction_id not the same as id sent")
	}

	return nil
}
