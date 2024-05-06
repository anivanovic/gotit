package util

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/bits-and-blooms/bitset"
)

type MessageType uint8

const (
	ChokeMessageType MessageType = iota
	UnchokeMessageType
	InterestedMessageType
	NotInterestedMessageType
	HaveMessageType
	BitfieldMessageType
	RequestMessageType
	PieceMessageType
	CancelMessageType

	// KeepaliveMessageType has only length without type.
	// This is fake type which is never actually used.
	KeepaliveMessageType MessageType = 99
)

type PeerMessage struct {
	len     uint32
	Type    MessageType
	payload []byte

	index       uint32
	offset      uint32
	blockLength uint32
}

type Message interface {
	Index() uint32
	Offset() uint32
	Payload() []byte
	Type() MessageType
}

var KeepalivePeerMessage = &PeerMessage{
	len:  0,
	Type: KeepaliveMessageType,
}

// NewPeerMessage constructs PeerMessage from single message read from network
// connection. Data byte array should not contain length parameter as first byte.
func NewPeerMessage(data []byte) *PeerMessage {
	if len(data) == 0 {
		return KeepalivePeerMessage
	}

	msg := &PeerMessage{
		len:     uint32(len(data)),
		Type:    MessageType(data[0]),
		payload: data[1:],
	}
	data = msg.payload

	switch msg.Type {
	case HaveMessageType:
		msg.index = binary.BigEndian.Uint32(data[:4])
	case RequestMessageType, CancelMessageType:
		msg.index = binary.BigEndian.Uint32(msg.payload[:4])
		msg.offset = binary.BigEndian.Uint32(msg.payload[4:8])
		msg.blockLength = binary.BigEndian.Uint32(msg.payload[8:12])
	case PieceMessageType:
		msg.index = binary.BigEndian.Uint32(msg.payload[:4])
		msg.offset = binary.BigEndian.Uint32(msg.payload[4:8])

	default:
		// no op
	}

	return msg
}

func (m PeerMessage) Send(w io.Writer) (int, error) {
	// TODO: cache buffer object in a pool
	buf := &bytes.Buffer{}
	writeBigEndian(buf, m.len)
	if m.Type == KeepaliveMessageType {
		return w.Write(buf.Bytes())
	}

	writeBigEndian(buf, m.Type)
	buf.Write(m.payload)

	return w.Write(buf.Bytes())
}

func (m PeerMessage) Index() uint32 {
	return m.index
}

func (m PeerMessage) Offset() uint32 {
	return m.offset
}

func (m PeerMessage) BlockLength() uint32 {
	return m.blockLength
}

func (m PeerMessage) Data() []byte {
	if m.Type != PieceMessageType {
		return nil
	}

	return m.payload[8:]
}

func (m PeerMessage) Bitfield() *bitset.BitSet {
	if m.Type != BitfieldMessageType {
		return nil
	}

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

func createNotInterestedMessage() *PeerMessage {
	return createSignalMessage(NotInterestedMessageType)
}

func CreateInterestedMessage() *PeerMessage {
	return createSignalMessage(InterestedMessageType)
}

func CreateChokeMessage() *PeerMessage {
	return createSignalMessage(ChokeMessageType)
}

func CreateUnchokeMessage() *PeerMessage {
	return createSignalMessage(UnchokeMessageType)
}

func createSignalMessage(code MessageType) *PeerMessage {
	return &PeerMessage{
		len:  1,
		Type: code,
	}
}

func createBitfieldMessage(b *bitset.BitSet) *PeerMessage {
	buf := &bytes.Buffer{}
	writeBigEndian(buf, uint32(b.Len()+1))
	writeBigEndian(buf, BitfieldMessageType)
	writeBigEndian(buf, b.Bytes())

	msg := &PeerMessage{
		len:     uint32(b.Len() + 1),
		Type:    BitfieldMessageType,
		payload: buf.Bytes(),
	}

	return msg
}

func createHaveMessage(index uint32) *PeerMessage {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, index)

	msg := &PeerMessage{
		len:     5,
		Type:    HaveMessageType,
		payload: payload,
	}

	return msg
}

func createCancelMessage(index, offset, blockLength uint32) *PeerMessage {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload, index)
	binary.BigEndian.PutUint32(payload, offset)
	binary.BigEndian.PutUint32(payload, blockLength)

	msg := &PeerMessage{
		len:         13,
		Type:        CancelMessageType,
		payload:     payload,
		index:       index,
		offset:      offset,
		blockLength: blockLength,
	}

	return msg
}

func CreatePieceMessage(index, offset, blockLength uint32) *PeerMessage {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload, index)
	binary.BigEndian.PutUint32(payload, offset)
	binary.BigEndian.PutUint32(payload, blockLength)

	msg := PeerMessage{
		len:         13,
		Type:        RequestMessageType,
		payload:     payload,
		index:       index,
		offset:      offset,
		blockLength: blockLength,
	}
	return &msg
}

func writeBigEndian(dest io.Writer, data any) {
	_ = binary.Write(dest, binary.BigEndian, data)
}
