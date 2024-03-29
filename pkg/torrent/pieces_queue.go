package torrent

import (
	"sync"
)

type PiecesQueue struct {
	failedMu       sync.Mutex
	failedMessages [][]byte
}

func NewPiecesQueue() *PiecesQueue {
	return &PiecesQueue{
		failedMu:       sync.Mutex{},
		failedMessages: make([][]byte, 0, 100),
	}
}

func (pq *PiecesQueue) RequestFailed(req []byte) {
	pq.failedMu.Lock()
	pq.failedMessages = append(pq.failedMessages, req)
	pq.failedMu.Unlock()
}

func (pq *PiecesQueue) FailedPieceMessage() []byte {
	pq.failedMu.Lock()
	defer pq.failedMu.Unlock()

	if len(pq.failedMessages) == 0 {
		return nil
	}
	req := pq.failedMessages[len(pq.failedMessages)-1]
	pq.failedMessages = pq.failedMessages[:len(pq.failedMessages)-1]
	return req
}
