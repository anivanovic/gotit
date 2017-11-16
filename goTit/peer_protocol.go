package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
	
	"github.com/anivanovic/goTit/bitset"
)

const (
	choke         = iota // 0
	unchoke              // 1
	interested           // 2
	notInterested        // 3
	have                 // 4
	bitfield             // 5
	request              // 6
	piece                // 7
	cancel               // 8
)

const blockLength uint32 = 16 * 1024

type peerMessage struct {
	size    uint32
	code    uint8
	payload []byte
}

func NewPeerMessage(data []byte) *peerMessage {
	message := peerMessage{size: uint32(len(data)), code: data[0], payload: data[1:]}
	return &message
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
		string(handshake[1:20]) != string(BITTORENT_PROT[:len(BITTORENT_PROT)]) ||
		reservedBytes != 0 ||
		bytes.Compare(sentHash, hash) != 0 ||
		bytes.Compare(sentPeerId, peerId) != 0
}

type Peer struct {
	Url     string
	Conn    net.Conn
	Torrent *Torrent
	Bitset  *bitset.BitSet
	Status  *PeerStatus
}

type PeerStatus struct {
	Choking    bool
	Interested bool
}

func newPeerStatus() *PeerStatus {
	return &PeerStatus{true, false}
}

func NewPeer(ip string, torrent *Torrent) *Peer {
	return &Peer{ip, nil, torrent, bitset.NewBitSet(torrent.PiecesNum), newPeerStatus()}
}

func (peer Peer) connect() {
	if peer.Conn == nil {
		conn, err := net.DialTimeout("tcp", peer.Url, time.Millisecond*1000)
		CheckError(err)
		peer.Conn = conn
	}
}

func (peer Peer) Announce(peerId []byte) bool {
	peer.connect()

	fmt.Println("writing to tcp socket")
	peer.Conn.SetDeadline(time.Now().Add(time.Second * 1))

	handshake := peer.Torrent.CreateHandshake(peerId)
	peer.Conn.Write(handshake)
	fmt.Println(len(handshake), "bytes written")

	response := readConn(peer.Conn)

	read := len(response)
	fmt.Println("Read all data", read)
	valid := checkHandshake(response, peer.Torrent.Hash, peerId)
	if valid {
		messages := readResponse(response[68:])
	}

	return valid
}

// Intended to be run in separate goroutin. Communicates with remote peer
// and downloads torrent
func (peer Peer) GoMessaging() {
	interestedM := createInterestedMessage()
	fmt.Println("Sending interested message")

	peer.Conn.SetDeadline(time.Now().Add(timeout))
	peer.Conn.Write(interestedM)

	fmt.Println("Reading Response")
	//WAIT:
	response = readConn(peer.Conn)

	// keepalive message
	//if len(response) == 0 {
	//	time.Sleep(time.Minute * 1)
	//	goto WAIT
	//}

	fmt.Println("Read all data", len(response))
	for i := 0; len(response) == 0 && i < 5; i++ {
		time.Sleep(timeout)
		response = readConn(peer.Conn)
	}
	if len(response) == 0 {
		continue
	}
	peerMessages := readResponse(response)

	message := peerMessages[0]
	if message.code == unchoke {
		for i := 0; i < 32; i++ {
			fmt.Print("\rRequesting piece 0 and block", i)
			peer.Conn.SetDeadline(time.Now().Add(timeout))
			peer.Conn.Write(createRequestMessage(0, i*int(blockLength)))

			response = readConn(peer.Conn)
			for i := 0; len(response) == 0 && i < 5; i++ {
				time.Sleep(time.Second * 5)
				response = readConn(peer.Conn)
			}
			if len(response) == 0 {
				continue IP_LOOP
			}
			readPieceResponse(response, peer.Conn)
		}
	}
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

func (peer Peer) handlePeerMesssage(message peerMessage) {
	switch message.code {
	case bitfield:
		peer.Bitset.InternalSet = message.payload
	case have:
		indx = uint32(message.payload)
		peer.Bitset.Set(indx)
	case interested:
		peer.Status.Interested = true
	case notInterested:
		peer.Status.Interested = false
	case choke:
		peer.Status.Choking = true
	case unchoke:
		peer.Status.Choking = false
	case request:
		fmt.Println("Peer", peer.Url, "requested piece")
	case piece:
	
	case cancel:
		fmt.Println("Peer", peer.Url, "cancled requested piece")
	}
}
