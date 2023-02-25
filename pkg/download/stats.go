package download

import (
	"sync/atomic"
	"time"
)

const mb = 1 << 20

type Stats struct {
	start time.Time

	download uint64
	upload   uint64
	left     uint64
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

func (ts Stats) Speed() float64 {
	dur := time.Since(ts.start)
	return float64(ts.Download()) / dur.Seconds() / mb
}
