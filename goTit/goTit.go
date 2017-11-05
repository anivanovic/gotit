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

const (
	letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	blockLength uint32 = 16 * 1024
	listenPort uint16 = 8999
)

var BITTORENT_PROT = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}


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
		conn.SetDeadline(time.Now().Add(timeout))
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

	conn.SetDeadline(time.Now().Add(timeout))
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

func main() {
	torrentContent, _ := ioutil.ReadFile("C:/Users/eaneivc/Downloads/[zooqle.com] Wonder Women 2017 HC HDRip 720p.torrent")
	fmt.Println("-------------------------------------------------------------------------------------")
	torrentString := string(torrentContent)
	_, benDict := metainfo.Parse(torrentString)
	fmt.Println(benDict.String())
	torrent := new(Torrent)
	torrent.Announce = benDict.Value("announce").String()
	torrent.CreatedBy = benDict.Value("created by").String()
	torrent.PieceLength, _ = strconv.Atoi(benDict.Value("info.piece length").String())
	torrent.Name = benDict.Value("info.name").String()
	torrent.Pieces = []byte(benDict.Value("info.pieces").String())
	torrent.CreationDate = benDict.Value("").String()

	info := benDict.Value("info").Encode()
	sha := sha1.New()
	sha.Write([]byte(info))

	var hash []byte
	hash = sha.Sum(nil)

	transactionId := uint32(12345612)
	peerId := randStringBytes(20)

	trackers := benDict.Value("announce-list")
	trackersList, _ := trackers.(metainfo.ListElement)
	
	announceList := make([]string)
	for elem := range trackersList.List {
		elemList, _ := elem.(metainfo.ListElement)
		announceList = append(announceList, elemList.List[0])
	}
	torrent.Announce_list = announceList
	
	listElement := trackers.(metainfo.ListElement)
	listElement.List = append(listElement.List, benDict.Value("announce"))
	ips := make(map[string]bool)
	for _, tracker := range listElement.List {
		var trackerUrl string
		if listTracker, ok := tracker.(metainfo.ListElement); ok {
			trackerUrl = listTracker.List[0].String()
		} else {
			trackerUrl = tracker.String()
		}

		u, err := url.Parse(trackerUrl)
		CheckError(err)
		tracker_ips := announce(u, transactionId, hash, peerId)
		for k, v := range *tracker_ips {
			ips[k] = v
		}
	}

	handhake := createHandshake(hash, peerId)
	fmt.Println("peers size in pool", len(ips))

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

			conn.SetDeadline(time.Now().Add(timeout))
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
				time.Sleep(timeout)
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
					conn.SetDeadline(time.Now().Add(timeout))
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

func announce(u *url.URL, transactionId uint32, hash []byte, peerId []byte) *map[string]bool {
	tracker := Tracker(u)
	defer tracker.Close()
	connId := tracker.Handshake(transactionId)
	ips := tracker.Announce(connId, hash, transactionId, peerId)
	return ips
}
