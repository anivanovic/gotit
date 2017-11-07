package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	url2 "net/url"
	"strconv"
	"time"
)

const timeout = time.Millisecond * 2000
const protocol_id uint64 = 0x41727101980

type tracker struct {
	Url  *url2.URL
	Conn *net.UDPConn
	addr *net.UDPAddr
}

func Tracker(url *url2.URL) *tracker {
	addr, err := net.ResolveUDPAddr(url.Scheme, url.Host)
	CheckError(err)

	conn, err := net.ListenUDP(url.Scheme, nil)
	CheckError(err)

	tracker := tracker{url, conn, addr}
	return &tracker
}

func (t tracker) Handshake(transactionId uint32) uint64 {
	request := new(bytes.Buffer)
	var action uint32 = 0

	binary.Write(request, binary.BigEndian, protocol_id)
	binary.Write(request, binary.BigEndian, action)
	binary.Write(request, binary.BigEndian, transactionId)

	t.Conn.SetDeadline(time.Now().Add(timeout))
	t.Conn.WriteTo(request.Bytes(), t.addr)

	p := make([]byte, 16)
	length, _, err := t.Conn.ReadFromUDP(p)

	CheckError(err)

	fmt.Println("read response")
	if length == 16 {
		fmt.Println("Read 16 bites")
	}
	connVar := binary.BigEndian.Uint32(p[:4])
	transResp := binary.BigEndian.Uint32(p[4:8])
	connId := binary.BigEndian.Uint64(p[8:16])
	fmt.Println("response: ", connVar, transResp, connId)

	return connId
}

func createAnnounce(connId uint64, hash, peerId []byte) *bytes.Buffer {
	//TODO move torrent data to torrent file
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(1))
	binary.Write(request, binary.BigEndian, uint32(127545))
	binary.Write(request, binary.BigEndian, hash)
	binary.Write(request, binary.BigEndian, peerId)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, uint64(960989559))
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, uint32(0))
	binary.Write(request, binary.BigEndian, uint32(0))
	randKey := rand.Int31()
	binary.Write(request, binary.BigEndian, randKey)
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, listenPort)
	return request
}

func (t tracker) Announce(connId uint64, hash []byte, transactionId uint32, peerId []byte) *map[string]bool {
	request := createAnnounce(connId, hash, peerId)
	t.Conn.SetDeadline(time.Now().Add(timeout))
	t.Conn.WriteTo(request.Bytes(), t.addr)
	fmt.Println("Send announce")
	response := readConn(t.Conn)
	ips := readAnnounceResponse(response, transactionId)
	return &ips
}

func (t tracker) Close() {
	t.Conn.Close()
}

func readAnnounceResponse(response []byte, transaction_id uint32) map[string]bool {
	fmt.Println("Dohvaćeno podataka ", len(response))
	if len(response) < 21 {
		return nil
	}
	resCode := binary.BigEndian.Uint32(response[:4])
	transaction_id = binary.BigEndian.Uint32(response[4:8])
	interval := binary.BigEndian.Uint32(response[8:12])
	leachers := binary.BigEndian.Uint32(response[12:16])
	seaders := binary.BigEndian.Uint32(response[16:20])
	peerCount := (len(response) - 20) / 6
	peerAddresses := response[20:]

	ips := make(map[string]bool, 0)
	fmt.Println("Peer count ", peerCount)
	fmt.Println("response code ", resCode, transaction_id, interval, leachers, seaders)
	for read := 0; read < peerCount; read++ {
		byteMask := 6

		ipAddress := net.IPv4(peerAddresses[byteMask*read], peerAddresses[byteMask*read+1], peerAddresses[byteMask*read+2], peerAddresses[byteMask*read+3])
		port := binary.BigEndian.Uint16(peerAddresses[byteMask*read+4 : byteMask*read+6])
		ipAddr := ipAddress.String() + ":" + strconv.Itoa(int(port))
		ips[ipAddr] = true
	}
	return ips
}
