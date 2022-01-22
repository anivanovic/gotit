package gotit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go"
	"github.com/bits-and-blooms/bitset"
	"github.com/tevino/abool/v2"
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
	peerNum    int
	listenPort int

	torrent       *Torrent
	torrentStatus torrentStatus

	poolMu   sync.Mutex
	peerPool map[string]*Peer

	failedMu       sync.Mutex
	failedMessages [][]byte

	cancleCtx context.CancelFunc
	wg        *sync.WaitGroup

	done   *abool.AtomicBool
	doneCh chan struct{}
}

func NewMng(torrent *Torrent, peerNum, listenPort int) *torrentManager {
	return &torrentManager{
		torrent:        torrent,
		peerPool:       make(map[string]*Peer, 100),
		failedMessages: make([][]byte, 0, 100),
		peerNum:        peerNum,
		listenPort:     listenPort,
		poolMu:         sync.Mutex{},
		failedMu:       sync.Mutex{},
		wg:             &sync.WaitGroup{},
		doneCh:         make(chan struct{}),
		done:           abool.New(),
		torrentStatus: torrentStatus{
			download: 0,
			upload:   0,
			left:     uint64(torrent.Length)}}
}

func (mng *torrentManager) Download() error {
	var ctx context.Context
	ctx, mng.cancleCtx = context.WithCancel(context.Background())

	mng.initStatisticsPrinting()
	mng.getIps(ctx)

	mng.waitDone()
	mng.waitPeers()
	return nil // TODO: propagate errors
}

// announce to all trackers from torrent file and gather
// peers ip addresses
func (mng *torrentManager) getIps(ctx context.Context) {
	log.Info("trackers", zap.Any("urls", mng.torrent.Trackers))

	for url := range mng.torrent.Trackers {
		go mng.runTracker(ctx, url)
	}
}

func (mng *torrentManager) runTracker(ctx context.Context, url string) error {
	tracker, err := NewTracker(url)
	if err != nil {
		return err
	}

	for {
		log.Info("Sending announce to tracker", zap.String("url", url))

		ips, err := mng.announceToTracker(ctx, tracker)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}

			log.Error("tracker announce failed",
				zap.String("url", url),
				zap.Error(err))
		} else {
			log.Sugar().With("url", url).Infof("tracker sent %d peers", len(ips))
			go mng.initPeers(ctx, ips)
		}

		if err := tracker.WaitInterval(ctx); err != nil {
			return nil
		}
	}
}

func (mng *torrentManager) initPeers(ctx context.Context, ips []string) {
	for _, ip := range ips {
		peer := NewPeer(ip, mng)
		if mng.AddPeer(peer) {
			mng.startPeerDownload(ctx, peer)
		}
	}
}

func (mng *torrentManager) announceToTracker(ctx context.Context, tracker Tracker) ([]string, error) {
	var ips []string
	err := retry.Do(
		func() error {
			var err error
			ips, err = tracker.Announce(ctx, mng)
			return err
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Warn("failed tracker announce",
				zap.Error(err),
				zap.String("url", tracker.Url()),
				zap.Uint("attempt", n+1))
		}),
		retry.Attempts(5),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	)
	return ips, err
}

func (mng *torrentManager) startPeerDownload(ctx context.Context, peer *Peer) {
	err := retry.Do(
		func() error {
			return peer.Announce()
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Warn("failed peer announce. attempt",
				zap.Error(err),
				zap.String("ip", peer.Url),
				zap.Uint("attempt", n+1))
		}),
		retry.Attempts(5),
		retry.Delay(500),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	)

	if err != nil {
		log.Warn("error announcing to peer",
			zap.String("ip", peer.Url),
			zap.Error(err))
		return
	}

	mng.wg.Add(1)
	go func() {
		defer mng.wg.Done()
		peer.Run(ctx)
	}()
}

func (mng *torrentManager) initStatisticsPrinting() {
	go func() {
		for mng.done.IsNotSet() {
			fmt.Printf("Downloaded: %d, Left: %d, Peers: %d",
				mng.torrentStatus.Download()/mb,
				mng.torrentStatus.Left()/mb,
				len(mng.peerPool))
			fmt.Println()
			time.Sleep(time.Second * 10)
		}
	}()
}

func (mng *torrentManager) Close() {
	mng.cancleCtx()
	mng.waitPeers()
	mng.setDone()
}

func (mng *torrentManager) waitDone() {
	<-mng.doneCh
}

func (mng *torrentManager) waitPeers() {
	mng.wg.Wait()
}

func (mng *torrentManager) setDone() {
	if mng.done.SetToIf(false, true) {
		close(mng.doneCh)
	}
}

func (mng *torrentManager) AddPeer(peer *Peer) bool {
	mng.poolMu.Lock()
	defer mng.poolMu.Unlock()

	if mng.peerPool[peer.Url] != nil {
		return false
	}

	mng.peerPool[peer.Url] = peer
	return true
}

func (mng *torrentManager) UpdateStatus(downloaded, uploaded uint64) {
	mng.torrentStatus.AddDownload(downloaded)
	mng.torrentStatus.AddUpload(uploaded)
}

func (mgn *torrentManager) NextPieceRequest(bitset *bitset.BitSet) (uint, bool) {
	return mgn.torrent.CreateNextRequestMessage(bitset)
}

func (mng *torrentManager) RequestFailed(req []byte) {
	mng.failedMu.Lock()
	mng.failedMessages = append(mng.failedMessages, req)
	mng.failedMu.Unlock()

	log.Warn("Piece request faild")
	log.Debug("Peer request failed messages",
		zap.Int("size", len(mng.failedMessages)))
}

func (mng *torrentManager) FailedPieceMessage() []byte {
	mng.failedMu.Lock()
	defer mng.failedMu.Unlock()

	if len(mng.failedMessages) == 0 {
		return nil
	}
	req := mng.failedMessages[len(mng.failedMessages)-1]
	mng.failedMessages = mng.failedMessages[:len(mng.failedMessages)-1]
	return req
}
