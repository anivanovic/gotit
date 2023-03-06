package util

import "encoding/binary"

type PeerMessage struct {
	Size    uint32
	Code    uint8
	Payload []byte
}

func (m *PeerMessage) Index() uint32 {
	return binary.BigEndian.Uint32(m.Payload[:4])
}

func (m *PeerMessage) Offset() uint32 {
	return binary.BigEndian.Uint32(m.Payload[4:8])
}

func (m *PeerMessage) Data() []byte {
	return m.Payload[8:]
}
