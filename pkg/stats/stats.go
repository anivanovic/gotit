package stats

import (
	"math"
	"sync/atomic"
	"time"
)

const mb = 1 << 20

type Stats struct {
	start       time.Time
	torrentSize uint64

	download   uint64
	upload     uint64
	left       uint64
	peerNum    uint64
	trackerNum uint64
}

func NewStats(torrentSize uint64) *Stats {
	return &Stats{
		start:       time.Now(),
		torrentSize: torrentSize,
		download:    0,
		upload:      0,
		left:        torrentSize,
		peerNum:     0,
		trackerNum:  0,
	}
}

func (ts *Stats) AddDownload(size uint64) {
	atomic.AddUint64(&ts.download, size)
	atomic.AddUint64(&ts.left, -size)
}

func (ts *Stats) AddUpload(size uint64) {
	atomic.AddUint64(&ts.upload, size)
}

func (ts *Stats) Download() uint64 {
	return atomic.LoadUint64(&ts.download)
}

func (ts *Stats) Upload() uint64 {
	return atomic.LoadUint64(&ts.upload)
}

func (ts *Stats) Left() uint64 {
	return atomic.LoadUint64(&ts.left)
}

func (ts *Stats) Speed() float64 {
	dur := time.Since(ts.start)
	return float64(ts.Download()) / dur.Seconds() / mb
}

func (ts *Stats) AddPeer() {
	atomic.AddUint64(&ts.peerNum, 1)
}

func (ts *Stats) RemovePeer() {
	atomic.AddUint64(&ts.peerNum, ^uint64(0))
}

func (ts *Stats) PeerNum() uint64 {
	return atomic.LoadUint64(&ts.peerNum)
}

func (ts *Stats) AddTracker() {
	atomic.AddUint64(&ts.trackerNum, 1)
}

func (ts *Stats) RemoveTracker() {
	atomic.AddUint64(&ts.trackerNum, ^uint64(0))
}

func (ts *Stats) TrackerNum() uint64 {
	return atomic.LoadUint64(&ts.trackerNum)
}

func (ts *Stats) PercentCompleted() int {
	if ts.torrentSize == 0 || ts.Download() == 0 {
		return 0
	}

	return int(math.Round((float64(ts.Download()) / float64(ts.torrentSize)) * 100.0))
}
