package gotit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go"
	"github.com/bits-and-blooms/bitset"
	"go.uber.org/zap"
)

const (
	mb = 1 << 20
)

type torrentStatus struct {
	download uint64
	upload   uint64
	left     uint64
}

func (ts *torrentStatus) AddDownload(size uint64) {
	atomic.AddUint64(&ts.download, size)
	atomic.AddUint64(&ts.left, -size)
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
	ips            StringSet
	peerNum        int
	listenPort     int
	cancleCtx      context.CancelFunc
	wg             *sync.WaitGroup
	sync.Mutex
}

func NewMng(torrent *Torrent, peerNum, listenPort int) *torrentManager {
	return &torrentManager{
		torrent:        torrent,
		peerPool:       make(map[int]*Peer),
		failedMessages: make([][]byte, 0),
		ips:            NewStringSet(),
		peerNum:        peerNum,
		listenPort:     listenPort,
		Mutex:          sync.Mutex{},
		wg:             &sync.WaitGroup{},
		torrentStatus: torrentStatus{
			download: 0,
			upload:   0,
			left:     uint64(torrent.Length)}}
}

// announce to all trackers from torrent file and gather
// peers ip addresses
func (mng *torrentManager) getIps(ctx context.Context) error {
	wg := sync.WaitGroup{}
	for url := range mng.torrent.Trackers {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			log.Info("Sending announce to tracker", zap.String("url", url))
			ips, err := announceToTracker(ctx, url, mng)
			if err != nil {
				log.Error("tracker announce failed",
					zap.String("url", url),
					zap.Error(err))
				return
			}
			log.Sugar().With("url", url).Infof("tracker sent %d peers", len(ips))

			mng.Lock()
			defer mng.Unlock()
			mng.ips.AddAll(ips)
		}(url)
	}
	wg.Wait()

	return nil
}

func announceToTracker(ctx context.Context, url string, mng *torrentManager) (map[string]struct{}, error) {
	tracker, err := CreateTracker(url)
	if err != nil {
		return nil, err
	}
	defer tracker.Close()

	var ips map[string]struct{}
	err = retry.Do(
		func() error {
			var err error
			ips, err = tracker.Announce(ctx, mng)
			return err
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Warn("failed tracker announce",
				zap.Error(err),
				zap.String("url", url),
				zap.Uint("attempt", n+1))
		}),
		retry.Attempts(5),
		retry.Delay(time.Second*1),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	)
	return ips, err
}

func (mng *torrentManager) startDownload(ctx context.Context) error {
	indx := 0
	for ip := range mng.ips {
		if indx >= mng.peerNum {
			break
		}

		mng.wg.Add(1)
		go func(ip string) {
			peer := NewPeer(ip, mng.torrent, mng)
			err := retry.Do(
				func() error {
					return peer.Announce()
				},
				retry.LastErrorOnly(true),
				retry.OnRetry(func(n uint, err error) {
					log.Warn("failed peer announce. attempt",
						zap.Error(err),
						zap.String("ip", ip),
						zap.Uint("attempt", n+1))
				}),
				retry.Attempts(5),
				retry.Delay(500),
				retry.DelayType(retry.BackOffDelay),
				retry.Context(ctx),
			)
			if err != nil {
				log.Warn("error announcing to peer",
					zap.String("ip", ip),
					zap.Error(err))
				mng.wg.Done()
				return
			}

			mng.Lock()
			mng.AddPeer(peer)
			mng.Unlock()

			peer.GoMessaging(ctx, mng.wg)
		}(ip)
		indx++
	}

	go func() {
		for {
			time.Sleep(time.Second * 10)
			fmt.Printf("Downloaded: %d, Left: %d, Peers: %d",
				mng.torrentStatus.Download()/mb,
				mng.torrentStatus.Left()/mb,
				len(mng.peerPool))
			fmt.Println()
		}
	}()

	mng.wait()
	return nil // TODO: propagate errors
}

func (mng *torrentManager) Download() error {
	var ctx context.Context
	ctx, mng.cancleCtx = context.WithCancel(context.Background())

	if err := mng.getIps(ctx); err != nil {
		return err
	}

	return mng.startDownload(ctx)
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
	defer mng.Unlock()
	mng.failedMessages = append(mng.failedMessages, req)
	log.Warn("Piece request faild")
	log.Debug("Peer request failed messages",
		zap.Int("size", len(mng.failedMessages)))
}

func (mgn *torrentManager) NextPieceRequest(bitset *bitset.BitSet) (uint, bool) {
	return mgn.torrent.CreateNextRequestMessage(bitset)
}

func (mng *torrentManager) FailedPieceMessage() []byte {
	if len(mng.failedMessages) == 0 {
		return nil
	}
	req := mng.failedMessages[len(mng.failedMessages)-1]
	mng.failedMessages = mng.failedMessages[:len(mng.failedMessages)-1]
	return req
}
