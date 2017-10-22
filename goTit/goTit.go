package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"

	"bytes"

	"crypto/sha1"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/anivanovic/goTit/metainfo"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const blockLength = 2 ^ 14

var BITTORENT_PROT = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}

const peerPort = 8999

func CheckError(err error) {
	if err != nil {
		log.Printf("%T %+v", err, err)
	}
}

func randStringBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return b
}

func createRequestMessage(piece int, beginOffset int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(request))
	binary.Write(message, binary.BigEndian, uint32(piece))
	binary.Write(message, binary.BigEndian, uint32(beginOffset))
	binary.Write(message, binary.BigEndian, uint32(blockLength))

	return message.Bytes()
}

func createNotInterestedMessage() []byte {
	return createSignalMessage(notInterested)
}

func createInterestedMessage() []byte {
	return createSignalMessage(interested)
}

func createChokeMessage() []byte {
	return createSignalMessage(choke)
}

func createUnchokeMessage() []byte {
	return createSignalMessage(unchoke)
}

func createSignalMessage(code int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(1))
	binary.Write(message, binary.BigEndian, uint8(code))

	return message.Bytes()
}

func createBitfieldMessage() []byte {
	// TODO izmjeniti hardkodirane djelove
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(265))
	binary.Write(message, binary.BigEndian, uint8(bitfield))
	binary.Write(message, binary.BigEndian, [113]uint8{})

	return message.Bytes()
}

func createHaveMessage(pieceIdx int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(have))
	binary.Write(message, binary.BigEndian, uint32(pieceIdx))

	return message.Bytes()
}

func createCancleMessage(pieceIdx int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(cancel))
	binary.Write(message, binary.BigEndian, uint32(pieceIdx))

	return message.Bytes()
}

func readConn(conn net.Conn) []byte {
	response := make([]byte, 0, 4096)
	tmp := make([]byte, bytes.MinRead)

	for {
		conn.SetDeadline(time.Now().Add(time.Second * 5))
		n, err := conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		fmt.Println("Read data from peer ", n)
		response = append(response, tmp[:n]...)
	}

	return response
}

func readHandshake(conn net.Conn) []byte {
	response := make([]byte, 0, 68)

	conn.SetDeadline(time.Now().Add(time.Second * 5))
	n, err := conn.Read(response)
	if err != nil {
		CheckError(err)
		return response
	}
	fmt.Println("Read data from peer ", n)

	return response
}

func readResponse(response []byte) []peerMessage {
	read := len(response)
	currPossition := 0

	messages := make([]peerMessage, 0)
	for currPossition < read {
		size := int(binary.BigEndian.Uint32(response[currPossition : currPossition+4]))
		currPossition += 4
		fmt.Println("size", size)
		message := NewPeerMessage(response[currPossition : currPossition+size])
		fmt.Println("message type:", message.code)
		fmt.Printf("peer has the following peeces %b\n", message.payload)
		currPossition = currPossition + size
		messages = append(messages, *message)
	}

	return messages
}

func createHandshake(hash []byte, peerId []byte) []byte {
	request := new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, uint8(19))
	binary.Write(request, binary.BigEndian, BITTORENT_PROT)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, hash)
	binary.Write(request, binary.BigEndian, peerId)

	return request.Bytes()
}

func checkHandshake(handshake, hash, peerId []byte) bool {
	if len(handshake) < 68 {
		return false
	}

	ressCode := uint8(handshake[0])
	fmt.Println(ressCode, string(handshake[1:20]))
	reservedBytes := binary.BigEndian.Uint64(handshake[20:28])
	fmt.Println(reservedBytes)
	sentHash := handshake[28:48]
	fmt.Printf("info hash: %x\n", sentHash)
	sentPeerId := handshake[48:68]
	fmt.Printf("info hash: %b\n", sentPeerId)

	return ressCode != 19 ||
		string(handshake[1:20]) != "Bittorent protocol" ||
		reservedBytes != 0 ||
		bytes.Compare(sentHash, hash) != 0 ||
		bytes.Compare(sentPeerId, peerId) != 0
}

func main() {
	fileTor, _ := ioutil.ReadFile("C:/Users/eaneivc/Downloads/Wonder Woman (2017) [720p] [YTS.AG].torrent")
	fmt.Println("-------------------------------------------------------------------------------------")
	torrent := string(fileTor)
	_, benDict := metainfo.Parse(torrent)
	fmt.Println(benDict.GetData())

	infoDict := torrent[strings.Index(torrent, "4:info")+6 : len(torrent)-1]
	sha := sha1.New()
	sha.Write([]byte(string(infoDict)))

	var hash []byte
	hash = sha.Sum(nil)
	fmt.Printf("info hash: %x\n", hash)

	u, err := url.Parse(benDict.Value("announce"))
	CheckError(err)

	udpAddr, err := net.ResolveUDPAddr("udp", u.Host)
	fmt.Println("Connecting to: " + u.Host)
	CheckError(err)

	Conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 6679})
	CheckError(err)

	defer Conn.Close()

	request := new(bytes.Buffer)
	p := make([]byte, 16)

	var action uint32 = 0
	var connection_id uint64 = 0x41727101980
	transaction_id := uint32(12398636)

	binary.Write(request, binary.BigEndian, connection_id)
	binary.Write(request, binary.BigEndian, action)
	binary.Write(request, binary.BigEndian, transaction_id)

	Conn.SetDeadline(time.Now().Add(time.Second * time.Duration(5)))
	Conn.WriteTo(request.Bytes(), udpAddr)
	length, _, err := Conn.ReadFromUDP(p)

	CheckError(err)

	fmt.Println("read response")
	if length == 16 {
		fmt.Println("Read 16 bites")
	}
	connVar := binary.BigEndian.Uint32(p[:4])
	transResp := binary.BigEndian.Uint32(p[4:8])
	connId := binary.BigEndian.Uint64(p[8:16])
	fmt.Println("rsponse: ", connVar, transResp, connId)

	request = new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(1))
	binary.Write(request, binary.BigEndian, uint32(127545))
	binary.Write(request, binary.BigEndian, hash)
	peerId := randStringBytes(20)
	binary.Write(request, binary.BigEndian, peerId)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, uint64(960989559))
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, uint32(0))
	binary.Write(request, binary.BigEndian, uint32(0))
	randKey := rand.Int31()
	binary.Write(request, binary.BigEndian, randKey)
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, uint16(6679))

	Conn.SetDeadline(time.Now().Add(time.Second * time.Duration(5)))
	Conn.WriteTo(request.Bytes(), udpAddr)
	fmt.Println("Send announce")
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	fmt.Println("reading")

	for {
		Conn.SetDeadline(time.Now().Add(time.Second * time.Duration(5)))
		n, err := Conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		response = append(response, tmp[:n]...)

	}
	fmt.Println("READ")
	fmt.Println("Dohvaćeno podataka ", len(response))

	resCode := binary.BigEndian.Uint32(response[:4])
	transaction_id = binary.BigEndian.Uint32(response[4:8])
	interval := binary.BigEndian.Uint32(response[8:12])
	leachers := binary.BigEndian.Uint32(response[12:16])
	seaders := binary.BigEndian.Uint32(response[16:20])
	peerCount := (len(response) - 20) / 6
	peerAddresses := response[20:]
	ports := make([]uint16, 0)
	ips := make([]net.IP, 0)
	fmt.Println("Peer count ", peerCount)
	fmt.Println("response code ", resCode, transaction_id, interval, leachers, seaders)

	for read := 0; read < peerCount; read++ {
		byteMask := 6

		ipAddress := net.IPv4(peerAddresses[byteMask*read], response[byteMask*read+1], response[byteMask*read+2], response[byteMask*read+3])
		port := binary.BigEndian.Uint16(response[byteMask*read+4 : byteMask*read+6])
		ports = append(ports, port)
		ips = append(ips, ipAddress)
	}

	//fmt.Println(ports)
	//fmt.Println(ips)

	//	for read := 0; read < len(ips); read++ {
	handhake := createHandshake(hash, peerId)

	//ip := ips[read]
	//port := ports[read]
	//tcpAddr, _ := net.ResolveTCPAddr("tcp", "92.36.128.234:20337")
	tcpAddr, _ := net.ResolveTCPAddr("tcp", "109.182.237.147:58261")
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	CheckError(err)
	time.Sleep(time.Second * 2)

	defer conn.Close()
	if conn != nil {
		fmt.Println("writing to tcp socket")
		conn.SetDeadline(time.Now().Add(time.Second * 5))
		conn.Write(handhake)
		fmt.Println(len(handhake), "bytes written")
	}

	time.Sleep(time.Second * 2)
	response = readConn(conn)

	read := len(response)
	fmt.Println("Read all data", read)
	valid := checkHandshake(response, hash, peerId)

	if !valid {
		return
	}

	readResponse(response[68:])

	interestedM := createInterestedMessage()
	fmt.Println("Sending interested message")

	conn.SetDeadline(time.Now().Add(time.Second * 5))
	conn.Write(interestedM)

	fmt.Println("ReadingResponse")
	response = readConn(conn)
	fmt.Println("Read all data", len(response))
	readResponse(response)

	//	for i := 0; i < 32; i++ {
	//		fmt.Print("\rRequesting piece 0 and block", i)
	//		conn.SetDeadline(time.Now().Add(time.Second * 5))
	//		conn.Write(createRequestMessage(0, i*blockLength))
	//		readResponse(readConn(conn))
	//	}
}
