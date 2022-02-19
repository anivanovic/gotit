package bencode

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TorrentFile struct {
	Announce     string   `ben:"announce"`
	AnnounceList []string `ben:"announce-list"`
	UrlList      []string `ben:"url-list"`
	Info         struct {
		Files []struct {
			Length int64    `ben:"length"`
			Path   []string `ben:"path"`
		} `ben:"files"`
		Length      int64  `ben:"length"`
		Name        string `ben:"name"`
		PieceLength int    `ben:"piece length"`
		Pieces      string `ben:"pieces"`
	} `ben:"info"`
	Comment      string `ben:"comment"`
	CreatedBy    string `ben:"created by"`
	CreationDate int64  `ben:"creation date"`
}

func readTorrentFile(name string) ([]byte, error) {
	f, err := os.Open(fmt.Sprintf("./testdata/%s", name))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

func TestUnmarshal(t *testing.T) {
	t.Parallel()

	arch, err := readTorrentFile("tears-of-steel.torrent")
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		data   []byte
		target interface{}
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Read full torrrent file",
			args: args{
				data:   arch,
				target: &TorrentFile{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := Unmarshal(tt.args.data, tt.args.target); (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}

			torrent := tt.args.target.(*TorrentFile)
			assert.Equal(t, "udp://tracker.leechers-paradise.org:6969", torrent.Announce)
			assert.Equal(t, "WebTorrent <https://webtorrent.io>", torrent.Comment)
			assert.Equal(t, "WebTorrent <https://webtorrent.io>", torrent.CreatedBy)
			assert.Equal(t, int64(1490916654), torrent.CreationDate)
			assert.Len(t, torrent.AnnounceList, 8)
			assert.Len(t, torrent.UrlList, 1)
			assert.NotNil(t, torrent.Info)
			info := torrent.Info
			assert.Equal(t, "Tears of Steel", info.Name)
			assert.Equal(t, int64(0), info.Length)
			assert.Equal(t, 524288, info.PieceLength)
			assert.NotEmpty(t, info.Pieces)
			assert.NotEmpty(t, info.Files)
			files := info.Files
			for _, f := range files {
				assert.NotZero(t, f.Length)
				assert.NotEmpty(t, f.Path)
			}
		})
	}
}

func TestUnmarshal_TargetNotPointer(t *testing.T) {
	t.Parallel()

	data, err := readTorrentFile("tears-of-steel.torrent")
	if err != nil {
		t.Fatal(err)
	}

	target := TorrentFile{}
	err = Unmarshal(data, target)
	assert.Error(t, err)
}

func TestUnmarshal_TargetNotPointerStruct(t *testing.T) {
	t.Parallel()

	data, err := readTorrentFile("tears-of-steel.torrent")
	if err != nil {
		t.Fatal(err)
	}

	target := []string{}
	err = Unmarshal(data, target)
	assert.Error(t, err)
}

func BenchmarkUnmarshal(b *testing.B) {
	data, err := readTorrentFile("tears-of-steel.torrent")
	if err != nil {
		b.Fatal(err)
	}
	torrent := TorrentFile{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Unmarshal(data, &torrent); err != nil {
			b.Fatal(err)
		}
	}
}
