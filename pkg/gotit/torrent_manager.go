package gotit

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

type torrentManager struct {
	torrent        *Torrent
	peerPool       map[int]*Peer
	failedMessages [][]byte
	monitor        *sync.Mutex
}

func NewMng(torrent *Torrent) *torrentManager {
	return &torrentManager{torrent, make(map[int]*Peer), make([][]byte, 0), new(sync.Mutex)}
}

func (mng *torrentManager) AddPeer(peer *Peer) bool {
	if mng.peerPool[peer.Id] != nil {
		return false
	}

	mng.peerPool[peer.Id] = peer
	return true
}

func (mng *torrentManager) RequestFailed(req []byte) {
	mng.failedMessages = append(mng.failedMessages, req)
	log.Warn("Piece request faild")
	log.WithField("size", len(mng.failedMessages)).Debug("Peer request failed messages")
}

func (mgn *torrentManager) NextRequest() []byte {
	mgn.monitor.Lock()
	if mgn.failedMessages != nil && len(mgn.failedMessages) > 0 {
		log.Debug("Next piece request given from failed messages pool")
		var req []byte
		req, mgn.failedMessages = mgn.failedMessages[0], mgn.failedMessages[1:]
		mgn.monitor.Unlock()
		return req
	} else {
		log.Debug("Next piece request created from torrent bitset")
		req := mgn.torrent.CreateNextRequestMessage()
		mgn.monitor.Unlock()
		return req
	}
}
