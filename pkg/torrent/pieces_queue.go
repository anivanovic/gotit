package torrent

import (
	"sync"

	"github.com/anivanovic/gotit/pkg/util"
)

type PiecesQueue struct {
	failedMu       sync.Mutex
	failedMessages []*util.PeerMessage
}

func NewPiecesQueue() *PiecesQueue {
	return &PiecesQueue{
		failedMu:       sync.Mutex{},
		failedMessages: make([]*util.PeerMessage, 0, 100),
	}
}

func (pq *PiecesQueue) RequestFailed(msg *util.PeerMessage) {
	pq.failedMu.Lock()
	pq.failedMessages = append(pq.failedMessages, msg)
	pq.failedMu.Unlock()
}

func (pq *PiecesQueue) FailedPieceMessage() *util.PeerMessage {
	pq.failedMu.Lock()
	defer pq.failedMu.Unlock()

	if len(pq.failedMessages) == 0 {
		return nil
	}
	req := pq.failedMessages[len(pq.failedMessages)-1]
	pq.failedMessages = pq.failedMessages[:len(pq.failedMessages)-1]
	return req
}
