package tracker_test

import (
	"github.com/anivanovic/gotit/pkg/tracker"
	"testing"
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
			_, err := tracker.NewTracker(tt.args.urlString)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTracker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
