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
	"time"

	"strconv"

	"github.com/anivanovic/goTit/metainfo"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const blockLength uint32 = 16 * 1024

var BITTORENT_PROT = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}

const listenPort uint16 = 8999

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
	binary.Write(message, binary.BigEndian, uint32(13))
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
	tmp := make([]byte, 4096)

	for {
		conn.SetDeadline(time.Now().Add(time.Second * 5))
		n, err := conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		fmt.Println("Read data from ", n)
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

func readPieceResponse(response []byte, conn net.Conn) {
	currPossition := 0

	size := int(binary.BigEndian.Uint32(response[currPossition : currPossition+4]))
	currPossition += 4
	fmt.Println("size", size)
	message := NewPeerMessage(response[currPossition : currPossition+size])
	fmt.Println("message type:", message.code)

	indx := binary.BigEndian.Uint32(message.payload[:4])
	offset := binary.BigEndian.Uint32(message.payload[4:8])
	fmt.Println("index", indx, "offset", offset)
	fmt.Printf("payload %b\n", message.payload[8:])

	pieceSize := len(message.payload[8:])
	for pieceSize != int(blockLength) {
		response = readConn(conn)
		pieceSize += len(response)
		fmt.Printf("payload %b\n", response)
	}
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

func main() {
	torrentContent, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/Wonder Woman (2017) [720p] [YTS.AG].torrent")
	fmt.Println("-------------------------------------------------------------------------------------")
	torrent := string(torrentContent)
	_, benDict := metainfo.Parse(torrent)
	fmt.Println(benDict.String())

	info := benDict.Value("info").Encode()
	sha := sha1.New()
	sha.Write([]byte(info))

	var hash []byte
	hash = sha.Sum(nil)
	fmt.Printf("info hash: %x\n", hash)

	u, err := url.Parse(benDict.Value("announce").String())
	CheckError(err)

	udpAddr, err := net.ResolveUDPAddr("udp", u.Host)
	fmt.Println("Connecting to: " + u.Host)
	CheckError(err)

	Conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 6679})
	CheckError(err)

	defer Conn.Close()

	transactionId := uint32(12345612)
	tracker := new(Tracker)
	connId := tracker.Handshake(transactionId, Conn, udpAddr)

	peerId := randStringBytes(20)
	ips := tracker.Announce(connId, hash, Conn, udpAddr, transactionId, peerId)

	handhake := createHandshake(hash, peerId)

IP_LOOP:
	for ip, _ := range ips {

		conn, err := net.DialTimeout("tcp", ip, time.Millisecond*500)
		CheckError(err)

		if conn != nil {
			defer conn.Close()

			fmt.Println("writing to tcp socket")
			conn.SetDeadline(time.Now().Add(time.Second * 1))
			conn.Write(handhake)
			fmt.Println(len(handhake), "bytes written")

			response := readConn(conn)

			read := len(response)
			fmt.Println("Read all data", read)
			valid := checkHandshake(response, hash, peerId)

			if !valid {
				continue
			}

			readResponse(response[68:])

			interestedM := createInterestedMessage()
			fmt.Println("Sending interested message")

			conn.SetDeadline(time.Now().Add(time.Second * 5))
			conn.Write(interestedM)

			fmt.Println("Reading Response")
			//WAIT:
			response = readConn(conn)

			// keepalive message
			//if len(response) == 0 {
			//	time.Sleep(time.Minute * 1)
			//	goto WAIT
			//}

			fmt.Println("Read all data", len(response))
			for i := 0; len(response) == 0 && i < 5; i++ {
				time.Sleep(time.Second * 5)
				response = readConn(conn)
			}
			if len(response) == 0 {
				continue
			}
			peerMessages := readResponse(response)

			message := peerMessages[0]
			if message.code == unchoke {
				for i := 0; i < 32; i++ {
					fmt.Print("\rRequesting piece 0 and block", i)
					conn.SetDeadline(time.Now().Add(time.Second * 5))
					conn.Write(createRequestMessage(0, i*int(blockLength)))

					response = readConn(conn)
					for i := 0; len(response) == 0 && i < 5; i++ {
						time.Sleep(time.Second * 5)
						response = readConn(conn)
					}
					if len(response) == 0 {
						continue IP_LOOP
					}
					readPieceResponse(response, conn)
				}
			}
		}
	}
}
