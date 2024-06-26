package download

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/anivanovic/gotit"

	"github.com/anivanovic/gotit/pkg/peer"
	"github.com/anivanovic/gotit/pkg/stats"
	"github.com/anivanovic/gotit/pkg/torrent"
	"github.com/anivanovic/gotit/pkg/tracker"
	"github.com/anivanovic/gotit/pkg/util"

	"github.com/avast/retry-go"
	"go.uber.org/zap"
)

type Manager struct {
	logger     *zap.Logger
	peerNum    int
	listenPort int

	torrent         *torrent.Torrent
	torrentStatus   *stats.Stats
	progressPrinter *stats.ProgressPrinter

	poolMu   sync.Mutex
	peerPool map[string]*peer.Peer

	cancelCtx context.CancelFunc
	wg        *sync.WaitGroup
}

func NewMng(torrent *torrent.Torrent, logger *zap.Logger, peerNum, listenPort int) *Manager {
	s := stats.NewStats(uint64(torrent.Length))
	pp := stats.NewProgressPrinter(s)
	return &Manager{
		logger:          logger,
		torrent:         torrent,
		peerPool:        make(map[string]*peer.Peer, 100),
		peerNum:         peerNum,
		listenPort:      listenPort,
		poolMu:          sync.Mutex{},
		wg:              &sync.WaitGroup{},
		torrentStatus:   s,
		progressPrinter: pp,
	}
}

func (m *Manager) Download(ctx context.Context) error {
	ctx, m.cancelCtx = context.WithCancel(ctx)

	pieceCh := make(chan *util.PeerMessage, 1024)

	m.initStatisticsPrinting(ctx)
	m.getIps(ctx, pieceCh)

	go m.torrent.WritePiece(pieceCh, m.torrentStatus)

	m.waitPeers()
	<-ctx.Done()
	close(pieceCh)
	return nil // TODO: propagate errors
}

// announce to all trackers from torrent file and gather
// peers ip addresses
func (m *Manager) getIps(ctx context.Context, pieceCh chan *util.PeerMessage) {
	m.logger.Info("trackers", zap.Any("urls", m.torrent.Trackers))

	for url := range m.torrent.Trackers {
		go m.runTracker(ctx, url, pieceCh)
	}
}

func (m *Manager) runTracker(ctx context.Context, url string, pieceCh chan *util.PeerMessage) error {
	tracker, err := tracker.New(url, m.logger)
	if err != nil {
		return err
	}

	for {
		m.logger.Info("Sending announce to tracker", zap.String("url", url))

		ips, err := m.announceToTracker(ctx, tracker)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}

			m.logger.Error("tracker announce failed",
				zap.String("url", url),
				zap.Error(err))
		} else {
			m.logger.Sugar().With("url", url).Infof("tracker sent %d peers", len(ips))
			m.initPeers(ctx, ips, pieceCh)
		}

		if err := tracker.WaitInterval(ctx); err != nil {
			return nil
		}
	}
}

func (m *Manager) initPeers(ctx context.Context, ips []netip.AddrPort, pieceCh chan *util.PeerMessage) {
	pq := torrent.NewPiecesQueue()
	for _, ip := range ips {
		p := peer.NewPeer(ip, m.torrent, pq, pieceCh, m.logger)
		if m.AddPeer(p) {
			m.startPeerDownload(ctx, p)
		}
	}
}

func (m *Manager) announceToTracker(ctx context.Context, t gotit.Tracker) ([]netip.AddrPort, error) {
	var ips []netip.AddrPort
	err := retry.Do(
		func() error {
			var err error
			announceData := gotit.AnnounceData{
				Downloaded: m.torrentStatus.Download(),
				Uploaded:   m.torrentStatus.Upload(),
				Left:       m.torrentStatus.Left(),
				Port:       m.listenPort,
			}
			ips, err = t.Announce(ctx, string(m.torrent.Hash), &announceData)
			return err
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			m.logger.Debug("failed tracker announce",
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

func (m *Manager) startPeerDownload(ctx context.Context, peer *peer.Peer) {
	err := retry.Do(
		func() error {
			return peer.Announce(m.torrent)
		},
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			m.logger.Debug("failed peer announce",
				zap.Uint("attempt", n+1),
				zap.Stringer("ip", peer.AddrPort),
				zap.Error(err))
		}),
		retry.Attempts(5),
		retry.Delay(500),
		retry.DelayType(retry.BackOffDelay),
		retry.Context(ctx),
	)

	if err != nil {
		m.logger.Error("error announcing to peer",
			zap.Stringer("ip", peer.AddrPort),
			zap.Error(err))
		m.torrentStatus.RemovePeer()
		return
	}

	m.wg.Add(1)
	go func() {
		peer.Run(ctx)
		m.wg.Done()
	}()
}

func (m *Manager) initStatisticsPrinting(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 1):
				m.progressPrinter.Print()
			}
		}
	}()
}

func (m *Manager) Stop() {
	if m.peerPool == nil {
		return
	}
	m.cancelCtx()

	m.poolMu.Lock()
	defer m.poolMu.Unlock()

	for _, p := range m.peerPool {
		if err := p.Close(); err != nil {
			m.logger.Error("closing peer", zap.Error(err))
		}
	}
	m.peerPool = nil
	m.progressPrinter.Close()
}

func (m *Manager) waitPeers() {
	m.wg.Wait()
}

func (m *Manager) AddPeer(peer *peer.Peer) bool {
	m.poolMu.Lock()
	defer m.poolMu.Unlock()

	if m.peerPool[peer.AddrPort.String()] != nil {
		return false
	}

	m.peerPool[peer.AddrPort.String()] = peer
	m.torrentStatus.AddPeer()
	return true
}

func (m *Manager) UpdateStatus(downloaded, uploaded uint64) {
	m.torrentStatus.AddDownload(downloaded)
	m.torrentStatus.AddUpload(uploaded)
}
