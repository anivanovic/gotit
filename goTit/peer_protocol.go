package main

import ()

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
