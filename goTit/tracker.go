package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"time"
)

const timeout = time.Second * 5
const protocol_id uint64 = 0x41727101980

type Tracker struct {
	url string
}

func (tracker Tracker) Handshake(transactionId uint32, Conn *net.UDPConn, addr *net.UDPAddr) uint64 {
	request := new(bytes.Buffer)
	var action uint32 = 0

	binary.Write(request, binary.BigEndian, protocol_id)
	binary.Write(request, binary.BigEndian, action)
	binary.Write(request, binary.BigEndian, transactionId)

	Conn.SetDeadline(time.Now().Add(timeout))
	Conn.WriteTo(request.Bytes(), addr)

	p := make([]byte, 16)
	length, _, err := Conn.ReadFromUDP(p)

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

func (tracker Tracker) Announce(connId uint64, hash []byte, Conn *net.UDPConn, udpAddr *net.UDPAddr, transactionId uint32, peerId []byte) map[string]bool {
	request := createAnnounce(connId, hash, peerId)
	Conn.SetDeadline(time.Now().Add(timeout))
	Conn.WriteTo(request.Bytes(), udpAddr)
	fmt.Println("Send announce")
	response := readConn(Conn)
	ips := readAnnounceResponse(response, transactionId)
	return ips
}
