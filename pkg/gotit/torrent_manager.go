package gotit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

type torrentStatus struct {
	download uint64
	upload   uint64
	left     uint64
}

func (ts *torrentStatus) AddDownload(size uint64) {
	atomic.AddUint64(&ts.download, size)
	atomic.AddUint64(&ts.left, ^(size - 1))
}

func (ts *torrentStatus) AddUpload(size uint64) {
	atomic.AddUint64(&ts.upload, size)
}

func (ts *torrentStatus) Download() uint64 {
	return atomic.LoadUint64(&ts.download)
}

func (ts *torrentStatus) Upload() uint64 {
	return atomic.LoadUint64(&ts.upload)
}

func (ts *torrentStatus) Left() uint64 {
	return atomic.LoadUint64(&ts.left)
}

type torrentManager struct {
	torrent        *Torrent
	torrentStatus  torrentStatus
	peerPool       map[int]*Peer
	failedMessages [][]byte
	ips            map[string]struct{}
	peerNum        int
	cancleCtx      context.CancelFunc
	wg             *sync.WaitGroup
	sync.Mutex
}

func NewMng(torrent *Torrent, peerNum int) *torrentManager {
	return &torrentManager{
		torrent:        torrent,
		peerPool:       make(map[int]*Peer),
		failedMessages: make([][]byte, 0),
		ips:            make(map[string]struct{}),
		peerNum:        peerNum,
		Mutex:          sync.Mutex{},
		torrentStatus:  torrentStatus{0, 0, uint64(torrent.left)}}
}

// announce to all trackers from torrent file and gather
// peers ip addresses
func (mng *torrentManager) getIps() error {
	wg := sync.WaitGroup{}
	for _, trackerUrl := range mng.torrent.Announce_list {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			tracker_ips, err := announceToTracker(url, mng.torrent)
			if err != nil {
				CheckError(err)
				return
			}

			mng.Lock()
			defer mng.Unlock()
			for k := range tracker_ips {
				mng.ips[k] = struct{}{}
			}
		}(trackerUrl)
	}
	wg.Wait()

	return nil
}

func announceToTracker(url string, torrent *Torrent) (map[string]struct{}, error) {
	tracker, err := CreateTracker(url)
	if err != nil {
		return nil, err
	}

	defer tracker.Close()
	ips, err := tracker.Announce(context.Background(), torrent)
	if err != nil {
		return nil, err
	}

	return ips, nil
}

func (mng *torrentManager) startDownload() error {
	var ctx context.Context
	ctx, mng.cancleCtx = context.WithCancel(context.Background())

	mng.wg = &sync.WaitGroup{}
	indx := 0
	for ip := range mng.ips {
		if indx >= mng.peerNum {
			break
		}

		mng.wg.Add(1)
		go func(ip string) {
			peer := NewPeer(ip, mng.torrent, mng)
			err := peer.Announce()
			if err != nil {
				CheckError(err)
				mng.wg.Done()
				return
			}
			peer.GoMessaging(ctx, mng.wg)
		}(ip)
		indx++
	}

	go func() {
		for {
			time.Sleep(time.Second * 10)
			log.Infof("Download status - Downloaded: %d, Left: %d", mng.torrentStatus.Download(), mng.torrentStatus.Left())
		}
	}()

	mng.wait()
	return nil // TODO: propagate errors
}

func (mng *torrentManager) Download() error {
	if err := mng.getIps(); err != nil {
		return err
	}

	return mng.startDownload()
}

func (mng *torrentManager) Close() {
	mng.cancleCtx()
	mng.wait()
}

func (mng *torrentManager) wait() {
	mng.wg.Wait()
}

func (mng *torrentManager) AddPeer(peer *Peer) bool {
	if mng.peerPool[peer.Id] != nil {
		return false
	}

	mng.peerPool[peer.Id] = peer
	return true
}

func (mng *torrentManager) UpdateStatus(downloaded, uploaded uint64) {
	mng.torrentStatus.AddDownload(downloaded)
	mng.torrentStatus.AddUpload(uploaded)
}

func (mng *torrentManager) RequestFailed(req []byte) {
	mng.Lock()
	mng.failedMessages = append(mng.failedMessages, req)
	mng.Unlock()
	log.Warn("Piece request faild")
	log.WithField("size", len(mng.failedMessages)).Debug("Peer request failed messages")
}

func (mgn *torrentManager) NextRequest() []byte {
	mgn.Lock()
	defer mgn.Unlock()

	var req []byte
	if mgn.failedMessages != nil && len(mgn.failedMessages) > 0 {
		log.Debug("Next piece request given from failed messages pool")
		req, mgn.failedMessages = mgn.failedMessages[0], mgn.failedMessages[1:]
	} else {
		log.Debug("Next piece request created from torrent bitset")
		req = mgn.torrent.CreateNextRequestMessage()
	}

	return req
}
