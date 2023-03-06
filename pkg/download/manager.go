package download

import (
	"context"
	"errors"
	"fmt"
	"github.com/anivanovic/gotit"
	"github.com/tevino/abool/v2"
	"net/netip"
	"runtime"
	"sync"
	"time"

	"github.com/anivanovic/gotit/pkg/peer"
	"github.com/anivanovic/gotit/pkg/torrent"
	"github.com/anivanovic/gotit/pkg/tracker"
	"github.com/anivanovic/gotit/pkg/util"

	"github.com/avast/retry-go"
	"go.uber.org/zap"
)

var log = zap.L()

type Manager struct {
	peerNum    int
	listenPort int

	torrent       *torrent.Torrent
	torrentStatus Stats

	poolMu   sync.Mutex
	peerPool map[string]*peer.Peer

	cancelCtx context.CancelFunc
	wg        *sync.WaitGroup

	done   *abool.AtomicBool
	doneCh chan struct{}
}

func NewMng(torrent *torrent.Torrent, peerNum, listenPort int) *Manager {
	return &Manager{
		torrent:    torrent,
		peerPool:   make(map[string]*peer.Peer, 100),
		peerNum:    peerNum,
		listenPort: listenPort,
		poolMu:     sync.Mutex{},
		wg:         &sync.WaitGroup{},
		doneCh:     make(chan struct{}),
		done:       abool.New(),
		torrentStatus: Stats{
			start:    time.Now(),
			download: 0,
			upload:   0,
			left:     uint64(torrent.Length)}}
}

func (mng *Manager) Download() error {
	var ctx context.Context
	ctx, mng.cancelCtx = context.WithCancel(context.Background())

	pieceCh := make(chan *util.PeerMessage, 1024)

	mng.initStatisticsPrinting()
	mng.getIps(ctx, pieceCh)

	go mng.torrent.WritePiece(pieceCh)

	runtime.Gosched()

	mng.waitDone()
	mng.waitPeers()
	close(pieceCh)
	return nil // TODO: propagate errors
}

// announce to all trackers from torrent file and gather
// peers ip addresses
func (mng *Manager) getIps(ctx context.Context, pieceCh chan *util.PeerMessage) {
	log.Info("trackers", zap.Any("urls", mng.torrent.Trackers))

	for url := range mng.torrent.Trackers {
		go mng.runTracker(ctx, url, pieceCh)
	}
}

func (mng *Manager) runTracker(ctx context.Context, url string, pieceCh chan *util.PeerMessage) error {
	tracker, err := tracker.NewTracker(url)
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
			go mng.initPeers(ctx, ips, pieceCh)
		}

		if err := tracker.WaitInterval(ctx); err != nil {
			return nil
		}
	}
}

func (mng *Manager) initPeers(ctx context.Context, ips []netip.AddrPort, pieceCh chan *util.PeerMessage) {
	pq := torrent.NewPiecesQueue()
	for _, ip := range ips {
		peer := peer.NewPeer(ip, mng.torrent, pq, pieceCh, mng.doneCh)
		if mng.AddPeer(peer) {
			mng.startPeerDownload(ctx, peer)
		}
	}
}

func (mng *Manager) announceToTracker(ctx context.Context, t gotit.Tracker) ([]netip.AddrPort, error) {
	var ips []netip.AddrPort
	err := retry.Do(
		func() error {
			var err error
			announceData := gotit.AnnounceData{
				Downloaded: mng.torrentStatus.download,
				Uploaded:   mng.torrentStatus.upload,
				Left:       mng.torrentStatus.left,
				Port:       mng.listenPort,
			}
			ips, err = t.Announce(ctx, mng.torrent, &announceData)
			return err
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Warn("failed tracker announce",
				zap.Error(err),
				zap.String("url", t.Url()),
				zap.Uint("attempt", n+1))
		}),
		retry.Attempts(5),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	)
	return ips, err
}

func (mng *Manager) startPeerDownload(ctx context.Context, peer *peer.Peer) {
	err := retry.Do(
		func() error {
			return peer.Announce(ctx, mng.torrent)
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Warn("failed peer announce. attempt",
				zap.Error(err),
				zap.String("ip", peer.AddrPort.String()),
				zap.Uint("attempt", n+1))
		}),
		retry.Attempts(5),
		retry.Delay(500),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	)

	if err != nil {
		log.Warn("error announcing to peer",
			zap.String("ip", peer.AddrPort.String()),
			zap.Error(err))
		return
	}

	mng.wg.Add(1)
	go func() {
		defer mng.wg.Done()
		peer.Run(ctx)
	}()
}

func (mng *Manager) initStatisticsPrinting() {
	go func() {
		for mng.done.IsNotSet() {
			fmt.Printf("\rDownloaded: %d, Left: %d, Peers: %d - Speed %f",
				mng.torrentStatus.Download()/mb,
				mng.torrentStatus.Left()/mb,
				len(mng.peerPool),
				mng.torrentStatus.Speed())
			time.Sleep(time.Second * 10)
		}
	}()
}

func (mng *Manager) Close() {
	mng.cancelCtx()
	mng.waitPeers()
	mng.setDone()
}

func (mng *Manager) waitDone() {
	<-mng.doneCh
}

func (mng *Manager) waitPeers() {
	mng.wg.Wait()
}

func (mng *Manager) setDone() {
	if mng.done.SetToIf(false, true) {
		close(mng.doneCh)
	}
}

func (mng *Manager) AddPeer(peer *peer.Peer) bool {
	mng.poolMu.Lock()
	defer mng.poolMu.Unlock()

	if mng.peerPool[peer.AddrPort.String()] != nil {
		return false
	}

	mng.peerPool[peer.AddrPort.String()] = peer
	return true
}

func (mng *Manager) UpdateStatus(downloaded, uploaded uint64) {
	mng.torrentStatus.AddDownload(downloaded)
	mng.torrentStatus.AddUpload(uploaded)
}
