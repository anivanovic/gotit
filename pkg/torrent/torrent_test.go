package torrent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTorrent_createTorrentFiles(t *testing.T) {
	dir, err := os.MkdirTemp(".", "torrent-*")
	if err != nil {
		t.Fatal("error creating tmp dir", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	torrent := Torrent{
		TorrentFiles: []TorrentFile{
			{
				Path:   []string{"1.txt"},
				Length: 1,
			},
			{
				Path:   []string{"2.txt"},
				Length: 2,
			},
		},
		Name:        "torrent",
		IsDirectory: true,
	}
	if err := torrent.createTorrentFiles(dir); err != nil {
		t.Fatal("error creating torrent files", err)
	}

	firstF := filepath.Join(dir, "torrent", "1.txt")
	secondf := filepath.Join(dir, "torrent", "2.txt")
	assert.FileExists(t, firstF)
	assert.FileExists(t, secondf)

	torrent.IsDirectory = false
	torrent.Name = "test.txt"
	torrent.createTorrentFiles(dir)
	file := filepath.Join(dir, torrent.Name)
	assert.FileExists(t, file)
}
