package torrent

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/anivanovic/gotit/pkg/stats"
	"github.com/tevino/abool/v2"

	"github.com/bits-and-blooms/bitset"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/util"
)

const BlockLength uint = 128 * 1024

type Torrent struct {
	logger       *zap.Logger
	Trackers     util.StringSet
	Hash         []byte
	Length       int
	PieceLength  int
	Pieces       []Piece
	PiecesNum    int
	TorrentFiles []bencode.TorrentFile
	OsFiles      []*os.File
	Name         string
	CreationDate int64
	CreatedBy    string
	Comment      string
	IsDirectory  bool

	Metadata *bencode.Metainfo

	numOfBlocks int

	requested   *bitset.BitSet
	requestedMu *sync.Mutex

	downloaded   *bitset.BitSet
	downloadedMu *sync.Mutex

	done   *abool.AtomicBool
	doneCh chan struct{}
}

func New(metainfo *bencode.Metainfo, downloadDir string, logger *zap.Logger) (*Torrent, error) {
	t := &Torrent{
		logger:       logger,
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
		Metadata:     metainfo,
	}
	t.logger.Debug("Created client id")
	t.Name = metainfo.Info.Name
	t.CreatedBy = metainfo.CreatedBy
	t.CreationDate = metainfo.CreationDate
	t.Comment = metainfo.Comment
	t.PieceLength = int(metainfo.Info.PieceLength)
	pieces, err := NewPieces([]byte(metainfo.Info.Pieces))
	if err != nil {
		return nil, err
	}
	t.Pieces = pieces
	t.numOfBlocks = t.PieceLength / int(BlockLength)
	t.Hash = metainfo.Hash()

	announce := metainfo.Announce
	announceSet := util.NewStringSet()
	announceSet.Add(announce)
	for _, el := range metainfo.AnnounceList {
		for _, e := range el {
			announceSet.Add(e)
		}
	}
	for _, el := range metainfo.UrlList {
		announceSet.Add(el)
	}
	t.Trackers = announceSet
	t.IsDirectory = metainfo.Info.Length == 0

	if !t.IsDirectory {
		t.Length = int(metainfo.Info.Length)
	} else {
		files := metainfo.Info.Files

		var completeLength = 0
		for _, file := range files {
			completeLength += file.Length
		}
		t.TorrentFiles = metainfo.Info.Files
		t.Length = completeLength
	}
	t.PiecesNum = int(math.Ceil(float64(t.Length) / float64(t.PieceLength)))
	t.requested = bitset.New(uint(t.PiecesNum))
	t.downloaded = bitset.New(uint(t.PiecesNum))

	if err := t.initDownloadDir(downloadDir); err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Torrent) SetDownloaded(pieceIndx uint) {
	t.downloadedMu.Lock()
	defer t.downloadedMu.Unlock()

	t.downloaded.Set(pieceIndx)
}

func (t *Torrent) Next(have *bitset.BitSet) (uint, bool) {
	t.requestedMu.Lock()
	defer t.requestedMu.Unlock()

	for i, exists := t.requested.NextClear(0); exists; i, exists = t.requested.NextClear(i + 1) {
		if !have.Test(i) {
			continue
		}

		t.requested.Set(i)
		return i, true
	}

	return 0, false
}

func (t *Torrent) Done() bool {
	t.downloadedMu.Lock()
	defer t.downloadedMu.Unlock()
	return t.downloaded.All()
}

func (t *Torrent) initDownloadDir(root string) error {
	path := filepath.Join(root, t.Name)
	var filePaths []string
	if t.IsDirectory {
		if err := os.Mkdir(path, os.ModePerm); err != nil && os.IsNotExist(err) {
			return err
		}

		for _, tf := range t.TorrentFiles {
			filePaths = append(filePaths, filepath.Join(path, tf.Path[0]))
		}
	} else {
		filePaths = append(filePaths, path)
	}

	for _, path := range filePaths {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		t.OsFiles = append(t.OsFiles, f)
	}

	return nil
}

func (t *Torrent) CheckPiece(data []byte, index int) bool {
	hasher := sha1.New()
	hasher.Write(data)
	hash := hasher.Sum(nil)

	return bytes.Equal(t.Pieces[index].sha1, hash)
}

func (t *Torrent) WritePiece(piecesCh <-chan *util.PeerMessage, stats *stats.Stats) {
	for msg := range piecesCh {
		piecePoss := int(msg.Index())*t.PieceLength + int(msg.Offset())
		stats.AddDownload(uint64(len(msg.Data())))

		if err := t.writePieceData(msg.Data(), piecePoss); err != nil {
			t.logger.Error("Failed to write piece",
				zap.Int("index", int(msg.Index())),
				zap.Int("offset", int(msg.Offset())),
				zap.Error(err))
			continue
		}

		if int(msg.Offset())+len(msg.Data()) == t.PieceLength {
			t.SetDownloaded(uint(msg.Index()))
		}
	}

	t.logger.Debug("Finished writing pieces")
}

// writePieceData writes data at the given absolute torrent byte position,
// spanning across multiple files as needed.
func (t *Torrent) writePieceData(data []byte, piecePoss int) error {
	if !t.IsDirectory {
		_, err := t.OsFiles[0].WriteAt(data, int64(piecePoss))
		return err
	}

	// Find the file where piece should be written to.
	fileIdx := 0
	for fileIdx < len(t.TorrentFiles) {
		if t.TorrentFiles[fileIdx].Length > piecePoss {
			break
		}
		piecePoss -= t.TorrentFiles[fileIdx].Length
		fileIdx++
	}
	if fileIdx >= len(t.TorrentFiles) {
		return errors.New("piece position beyond all torrent files")
	}

	// Write data, advancing to the next file whenever the current one is full.
	for len(data) > 0 {
		if fileIdx >= len(t.TorrentFiles) {
			return errors.New("data extends beyond torrent files")
		}

		available := t.TorrentFiles[fileIdx].Length - piecePoss
		toWrite := len(data)
		if toWrite > available {
			toWrite = available
		}

		t.logger.Debug("Writing to file",
			zap.String("file", t.TorrentFiles[fileIdx].Path[0]),
			zap.Int("position", piecePoss),
			zap.Int("bytes", toWrite))

		if _, err := t.OsFiles[fileIdx].WriteAt(data[:toWrite], int64(piecePoss)); err != nil {
			return err
		}

		data = data[toWrite:]
		piecePoss = 0
		fileIdx++
	}

	return nil
}

func (t *Torrent) BlockNum() int {
	return t.numOfBlocks
}

// Close torrent os files
func (t *Torrent) Close() error {
	var err error
	for _, f := range t.OsFiles {
		err = multierr.Append(err, f.Close())
	}
	return err
}

func (t *Torrent) EmptyBitset() *bitset.BitSet {
	return bitset.New(uint(t.PiecesNum))
}
