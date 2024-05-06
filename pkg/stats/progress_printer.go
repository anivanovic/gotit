package stats

import (
	"fmt"

	"code.cloudfoundry.org/bytefmt"
	"github.com/gosuri/uilive"
)

type ProgressPrinter struct {
	torrentStats *Stats
	w            *uilive.Writer
}

func NewProgressPrinter(stats *Stats) *ProgressPrinter {
	p := &ProgressPrinter{torrentStats: stats, w: uilive.New()}
	p.w.Start()
	return p
}

func (pp ProgressPrinter) Close() {
	pp.w.Stop()
}

func (pp ProgressPrinter) Print() {
	_, _ = fmt.Fprint(pp.w, progressBar(pp.torrentStats.PercentCompleted(), 20))
	_, _ = fmt.Fprintf(
		pp.w,
		" downloaded (%s), left (%s), speed (%.1f MB/s), peers (%d), trackers (%d)\n",
		bytefmt.ByteSize(pp.torrentStats.Download()),
		bytefmt.ByteSize(pp.torrentStats.Left()),
		pp.torrentStats.Speed(),
		pp.torrentStats.PeerNum(),
		pp.torrentStats.TrackerNum(),
	)
	pp.w.Flush()
}
