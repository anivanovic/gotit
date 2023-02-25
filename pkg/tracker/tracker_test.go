package tracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTracker(t *testing.T) {
	type args struct {
		urlString string
	}
	tests := []struct {
		name    string
		args    args
		want    Tracker
		wantErr bool
	}{
		{
			name: "http tracker",
			args: args{
				urlString: "http://www.announcer.com:8090/annunce",
			},
			want: &httpTracker{},
		},
		{
			name: "udp tracker",
			args: args{
				urlString: "udp://tracker.leechers-paradise.org:6969",
			},
			want: &udpTracker{},
		},
		{
			name: "Invalid tracker url",
			args: args{
				urlString: "tcp://tracker.leechers-paradise.org:6969",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewTracker(tt.args.urlString)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTracker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.IsType(t, tt.want, got)
		})
	}
}
