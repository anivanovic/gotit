package gotit

import (
	"sync/atomic"
	"time"
)

const (
	mb = 1 << 20
)

type torrentStatus struct {
	start time.Time

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

func (ts torrentStatus) Speed() float64 {
	dur := time.Since(ts.start)
	return float64(ts.Download()) / dur.Seconds() / mb
}
