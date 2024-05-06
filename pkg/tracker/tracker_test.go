package tracker_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anivanovic/gotit/pkg/logger"
	"github.com/anivanovic/gotit/pkg/tracker"
	"github.com/stretchr/testify/assert"
)

func TestNewTracker(t *testing.T) {
	type args struct {
		urlString string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "http tracker",
			args: args{
				urlString: "http://www.announcer.com:8090/annunce",
			},
		},
		{
			name: "https tracker",
			args: args{
				urlString: "https://www.announcer.com:8090/annunce",
			},
		},
		{
			name: "udp tracker",
			args: args{
				urlString: "udp://tracker.leechers-paradise.org:6969",
			},
		},
		{
			name: "Invalid tracker url",
			args: args{
				urlString: "tcp://tracker.leechers-paradise.org:6969",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, err := logger.NewCommandLine("info", "color")
			assert.NoError(t, err)
			_, err = tracker.New(tt.args.urlString, l)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestTrackerAnnounce(t *testing.T) {
	//tUrl := setUpTracker(t, func(w http.ResponseWriter, r *http.Request) {
	//	w.Write([]byte("what am I doing"))
	//})
	//logger := zap.NewNop()
	//tracker, err := tracker.New(tUrl, logger)
	//assert.NoError(t, err)

	//tracker.Announce(context.Background(), torrent.New(&bencode.Metainfo{
	//	Announce:     "",
	//	AnnounceList: nil,
	//	UrlList:      nil,
	//	Info: struct {
	//		Files       []bencode.TorrentFile `ben:"files,optional"`
	//		Length      int64                 `ben:"length,optional"`
	//		Name        string                `ben:"name"`
	//		PieceLength int64                 `ben:"piece length"`
	//		Pieces      string                `ben:"pieces"`
	//	}{},
	//	InfoDictRaw:  nil,
	//	Comment:      "",
	//	CreatedBy:    "",
	//	CreationDate: 0,
	//	Encoding:     "",
	//}, "", logger), &gotit.AnnounceData{
	//	Downloaded: 0,
	//	Uploaded:   0,
	//	Left:       0,
	//	Port:       0,
	//})
}

func setUpTracker(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(func() {
		s.Close()
	})

	return s.URL
}
