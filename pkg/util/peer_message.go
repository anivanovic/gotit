package util

type PeerMessage struct {
	Size    uint32
	Code    uint8
	Payload []byte
}
