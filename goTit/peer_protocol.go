package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
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
