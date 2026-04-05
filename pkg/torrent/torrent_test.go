package torrent

import (
	"crypto/sha1"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bits-and-blooms/bitset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anivanovic/gotit/pkg/bencode"
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
		TorrentFiles: []bencode.TorrentFile{
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
	if err := torrent.initDownloadDir(dir); err != nil {
		t.Fatal("error creating torrent files", err)
	}

	firstF := filepath.Join(dir, "torrent", "1.txt")
	secondf := filepath.Join(dir, "torrent", "2.txt")
	assert.FileExists(t, firstF)
	assert.FileExists(t, secondf)

	torrent.IsDirectory = false
	torrent.Name = "test.txt"
	torrent.initDownloadDir(dir)
	file := filepath.Join(dir, torrent.Name)
	assert.FileExists(t, file)
}

// --- helpers -----------------------------------------------------------------

// makeTorrent builds a minimal Torrent with correctly-sized bitsets for unit tests.
func makeTorrent(piecesNum int) *Torrent {
	return &Torrent{
		PiecesNum:    piecesNum,
		PieceLength:  int(BlockLength),
		requested:    bitset.New(uint(piecesNum)),
		downloaded:   bitset.New(uint(piecesNum)),
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
	}
}

// sha1Of returns the raw 20-byte SHA1 of data, ready for use with NewPieces.
func sha1Of(data []byte) []byte {
	h := sha1.Sum(data)
	return h[:]
}

// --- EmptyBitset -------------------------------------------------------------

func TestEmptyBitset_SizeEqualsPiecesNum(t *testing.T) {
	// EmptyBitset must be sized by PiecesNum (one bit per piece), not PieceLength
	// (which is the byte-size of a single piece and is orders of magnitude larger).
	const piecesNum = 42
	tor := makeTorrent(piecesNum)
	tor.PieceLength = 262144 // 256 KiB — typical, much larger than piecesNum

	bs := tor.EmptyBitset()

	assert.Equal(t, uint(piecesNum), bs.Len(),
		"EmptyBitset should allocate one bit per piece (PiecesNum=%d), not one bit per data byte (PieceLength=%d)",
		piecesNum, tor.PieceLength)
}

// --- Done --------------------------------------------------------------------

func TestDone_FalseWhenNoPiecesDownloaded(t *testing.T) {
	tor := makeTorrent(10)
	assert.False(t, tor.Done())
}

func TestDone_FalseWhenPartiallyDownloaded(t *testing.T) {
	tor := makeTorrent(4)
	tor.downloaded.Set(0)
	tor.downloaded.Set(1)
	assert.False(t, tor.Done())
}

func TestDone_TrueWhenAllPiecesDownloaded(t *testing.T) {
	const n = 5
	tor := makeTorrent(n)
	for i := uint(0); i < n; i++ {
		tor.downloaded.Set(i)
	}
	assert.True(t, tor.Done())
}

func TestDone_ZeroBitsetIsVacuouslyTrue(t *testing.T) {
	// Documents the ordering bug: when PiecesNum=0 at bitset init time,
	// the downloaded bitset has size 0 and All() returns true immediately,
	// making every torrent appear complete before anything is downloaded.
	tor := &Torrent{
		downloaded:   bitset.New(0),
		downloadedMu: &sync.Mutex{},
	}
	assert.True(t, tor.Done(),
		"zero-size downloaded bitset reports Done=true (vacuously true) — this is the symptom of the ordering bug")
}

// --- SetDownloaded -----------------------------------------------------------

func TestSetDownloaded_MarksPiece(t *testing.T) {
	tor := makeTorrent(8)
	tor.SetDownloaded(3)
	assert.True(t, tor.downloaded.Test(3))
}

func TestSetDownloaded_DoesNotMarkOtherPieces(t *testing.T) {
	tor := makeTorrent(8)
	tor.SetDownloaded(3)
	for i := uint(0); i < 8; i++ {
		if i != 3 {
			assert.False(t, tor.downloaded.Test(i), "piece %d should not be marked", i)
		}
	}
}

func TestSetDownloaded_AllPiecesLeadsToDone(t *testing.T) {
	const n = 3
	tor := makeTorrent(n)
	for i := uint(0); i < n; i++ {
		tor.SetDownloaded(i)
	}
	assert.True(t, tor.Done())
}

// --- Next --------------------------------------------------------------------

func TestNext_ReturnsNotFoundWhenPeerHasNothing(t *testing.T) {
	tor := makeTorrent(4)
	emptyHave := bitset.New(4)
	_, found := tor.Next(emptyHave)
	assert.False(t, found)
}

func TestNext_ReturnsPieceAvailableInPeerBitset(t *testing.T) {
	tor := makeTorrent(4)
	have := bitset.New(4)
	have.Set(2)

	idx, found := tor.Next(have)
	require.True(t, found)
	assert.Equal(t, uint(2), idx)
}

func TestNext_MarksPieceAsRequested(t *testing.T) {
	tor := makeTorrent(4)
	have := bitset.New(4)
	have.Set(1)

	tor.Next(have)
	assert.True(t, tor.requested.Test(1))
}

func TestNext_SkipsAlreadyRequestedPiece(t *testing.T) {
	tor := makeTorrent(4)
	have := bitset.New(4)
	have.Set(0)
	have.Set(1)
	tor.requested.Set(0) // already in flight

	idx, found := tor.Next(have)
	require.True(t, found)
	assert.Equal(t, uint(1), idx)
}

func TestNext_ReturnsNotFoundWhenAllRequested(t *testing.T) {
	const n = uint(3)
	tor := makeTorrent(int(n))
	have := bitset.New(n)
	for i := uint(0); i < n; i++ {
		have.Set(i)
		tor.requested.Set(i)
	}
	_, found := tor.Next(have)
	assert.False(t, found)
}

func TestNext_ZeroBitsetNeverFindsAPiece(t *testing.T) {
	// Documents the ordering bug: when PiecesNum=0 at bitset init time,
	// the requested bitset has size 0, NextClear finds nothing, and Next()
	// always returns false — no pieces are ever requested.
	tor := &Torrent{
		requested:   bitset.New(0),
		requestedMu: &sync.Mutex{},
	}
	have := bitset.New(4)
	have.Set(0)
	have.Set(1)

	_, found := tor.Next(have)
	assert.False(t, found,
		"zero-size requested bitset means Next() never returns a piece — symptom of the ordering bug")
}

// --- CheckPiece --------------------------------------------------------------

func TestCheckPiece_ValidData(t *testing.T) {
	data := []byte("hello world")
	pieces, err := NewPieces(sha1Of(data))
	require.NoError(t, err)

	tor := &Torrent{Pieces: pieces}
	assert.True(t, tor.CheckPiece(data, 0))
}

func TestCheckPiece_CorruptedData(t *testing.T) {
	data := []byte("hello world")
	pieces, err := NewPieces(sha1Of(data))
	require.NoError(t, err)

	tor := &Torrent{Pieces: pieces}
	assert.False(t, tor.CheckPiece([]byte("corrupted!!"), 0))
}

func TestCheckPiece_EmptyData(t *testing.T) {
	data := []byte{}
	pieces, err := NewPieces(sha1Of(data))
	require.NoError(t, err)

	tor := &Torrent{Pieces: pieces}
	assert.True(t, tor.CheckPiece(data, 0))
}
