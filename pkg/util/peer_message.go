package util

import (
	"bytes"
	"encoding/binary"

	"github.com/bits-and-blooms/bitset"
)

type MessageType int

const (
	// KeepaliveMessageType has only length without type.
	// This is fake type which is never actually used.
	KeepaliveMessageType MessageType = 99

	ChokeMessageType MessageType = iota
	UnchokeMessageType
	InterestedMessageType
	NotInterestedMessageType
	HaveMessageType
	BitfieldMessageType
	RequestMessageType
	PieceMessageType
	CancelMessageType
)

type PeerMessage struct {
	len    int
	Type   MessageType
	index  uint32
	offset uint32
	size   int

	payload []byte
}

func (m *PeerMessage) Index() uint32 {
	return m.index
}

func (m *PeerMessage) Offset() uint32 {
	return m.offset
}

func (m *PeerMessage) Data() []byte {
	return m.payload
}

func (m *PeerMessage) Bitfield() *bitset.BitSet {
	return createBitset(m.payload)
}

func createBitset(payload []byte) *bitset.BitSet {
	set := make([]uint64, 0)
	i := 0
	lenPayload := len(payload)
	for i+8 < lenPayload {
		data := binary.BigEndian.Uint64(payload[i : i+8])
		set = append(set, data)
		i += 8
	}
	if i < lenPayload {
		n := lenPayload - i
		missing := 8 - n
		data := payload[i:lenPayload]
		for i := 0; i < missing; i++ {
			data = append(data, 0)
		}
		last := binary.BigEndian.Uint64(data)
		set = append(set, last)
	}

	return bitset.From(set)
}

var keepalivePeerMessage = &PeerMessage{
	len:  0,
	Type: KeepaliveMessageType,
}

// NewPeerMessage constructs PeerMessage from single message read from network
// connection. Data byte array should not contain length parameter as first byte.
func NewPeerMessage(data []byte) *PeerMessage {
	if len(data) == 0 { // keepalive message
		return keepalivePeerMessage
	}
	msg := &PeerMessage{
		Type: MessageType(data[0]),
	}
	data = data[1:]
	msg.len = len(data)

	switch msg.Type {
	case HaveMessageType:
		msg.index = binary.BigEndian.Uint32(data[:4])
	case RequestMessageType, CancelMessageType:
		msg.index = binary.BigEndian.Uint32(data[:4])
		msg.offset = binary.BigEndian.Uint32(data[4:8])
		msg.size = int(data[2])
	case PieceMessageType:
		msg.index = binary.BigEndian.Uint32(data[:4])
		msg.offset = binary.BigEndian.Uint32(data[4:8])
		msg.payload = data[8:]
		msg.size = len(msg.payload)
	case BitfieldMessageType:
		msg.payload = data
	}

	return msg
}

func createNotInterestedMessage() []byte {
	return createSignalMessage(NotInterestedMessageType)
}

func CreateInterestedMessage() []byte {
	return createSignalMessage(InterestedMessageType)
}

func createChokeMessage() []byte {
	return createSignalMessage(ChokeMessageType)
}

func createUnchokeMessage() []byte {
	return createSignalMessage(UnchokeMessageType)
}

func createSignalMessage(code MessageType) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(1))
	binary.Write(message, binary.BigEndian, uint8(code))

	return message.Bytes()
}

func createBitfieldMessage(b *bitset.BitSet) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, b.Len()+1)
	binary.Write(message, binary.BigEndian, uint8(BitfieldMessageType))
	binary.Write(message, binary.BigEndian, b.Bytes())

	return message.Bytes()
}

func createHaveMessage(pieceIdx int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(HaveMessageType))
	binary.Write(message, binary.BigEndian, uint32(pieceIdx))

	return message.Bytes()
}

func createCancelMessage(pieceIdx int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(CancelMessageType))
	binary.Write(message, binary.BigEndian, uint32(pieceIdx))

	return message.Bytes()
}

func CreatePieceMessage(pieceIdx, beginOffset, blockLen uint32) []byte {
	message := &bytes.Buffer{}
	binary.Write(message, binary.BigEndian, uint32(13))
	binary.Write(message, binary.BigEndian, uint8(RequestMessageType))
	binary.Write(message, binary.BigEndian, pieceIdx)
	binary.Write(message, binary.BigEndian, beginOffset)
	binary.Write(message, binary.BigEndian, blockLen)

	return message.Bytes()
}
