package torrent

import (
	"crypto/sha1"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bits-and-blooms/bitset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/stats"
	"github.com/anivanovic/gotit/pkg/util"
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

// makePieceMsg builds a PeerMessage of PieceMessageType carrying the given data.
func makePieceMsg(index, offset uint32, data []byte) *util.PeerMessage {
	payload := make([]byte, 1+4+4+len(data))
	payload[0] = byte(util.PieceMessageType)
	binary.BigEndian.PutUint32(payload[1:5], index)
	binary.BigEndian.PutUint32(payload[5:9], offset)
	copy(payload[9:], data)
	return util.NewPeerMessage(payload)
}

// makeMultiFileTorrent creates a directory torrent with real temp files.
func makeMultiFileTorrent(t *testing.T, dir string, files []bencode.TorrentFile, pieceLength int) *Torrent {
	t.Helper()
	tor := &Torrent{
		IsDirectory:  true,
		Name:         "multi",
		PieceLength:  pieceLength,
		TorrentFiles: files,
		requested:    bitset.New(uint(len(files))),
		downloaded:   bitset.New(uint(len(files))),
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
		logger:       zap.NewNop(),
	}
	require.NoError(t, tor.initDownloadDir(dir))
	t.Cleanup(func() { tor.Close() })
	return tor
}

// readAt reads n bytes from f at the given offset.
func readAt(t *testing.T, f *os.File, offset int64, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	_, err := f.ReadAt(buf, offset)
	require.NoError(t, err)
	return buf
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

func TestNext_IteratesPastMultiplePiecesNotInPeerBitset(t *testing.T) {
	// Peer has only the last piece. The loop must walk through pieces 0–3
	// before finding piece 4. This validates the i+1 advancement in the loop.
	tor := makeTorrent(5)
	have := bitset.New(5)
	have.Set(4)

	idx, found := tor.Next(have)
	require.True(t, found)
	assert.Equal(t, uint(4), idx)
	assert.True(t, tor.requested.Test(4))
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

// --- WritePiece --------------------------------------------------------------

func TestWritePiece_SingleFile_WritesDataAtOffset(t *testing.T) {
	dir := t.TempDir()
	data := []byte("hello torrent")

	tor := &Torrent{
		IsDirectory:  false,
		Name:         "single.bin",
		PieceLength:  100,
		requested:    bitset.New(1),
		downloaded:   bitset.New(1),
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
		logger:       zap.NewNop(),
	}
	require.NoError(t, tor.initDownloadDir(dir))
	t.Cleanup(func() { tor.Close() })

	ch := make(chan *util.PeerMessage, 1)
	ch <- makePieceMsg(0, 0, data)
	close(ch)

	tor.WritePiece(ch, stats.NewStats(0))

	got := readAt(t, tor.OsFiles[0], 0, len(data))
	assert.Equal(t, data, got)
}

func TestWritePiece_MultiFile_PieceFitsInFirstFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte("fits")

	tor := makeMultiFileTorrent(t, dir, []bencode.TorrentFile{
		{Path: []string{"a.bin"}, Length: 10},
		{Path: []string{"b.bin"}, Length: 10},
	}, 10)

	ch := make(chan *util.PeerMessage, 1)
	ch <- makePieceMsg(0, 0, data) // piecePoss = 0; file a has 10 bytes free
	close(ch)

	tor.WritePiece(ch, stats.NewStats(0))

	assert.Equal(t, data, readAt(t, tor.OsFiles[0], 0, len(data)))
	// second file untouched — verify it's still empty
	info, err := tor.OsFiles[1].Stat()
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())
}

func TestWritePiece_MultiFile_PieceInSecondFile(t *testing.T) {
	// Piece index 1 with PieceLength=5 → piecePoss = 5.
	// Loop: file a (length 5) is NOT > 5 → subtract → piecePoss = 0.
	// Loop: file b (length 10) IS > 0 → break, write to b at offset 0.
	dir := t.TempDir()
	data := []byte("bbb")

	tor := makeMultiFileTorrent(t, dir, []bencode.TorrentFile{
		{Path: []string{"a.bin"}, Length: 5},
		{Path: []string{"b.bin"}, Length: 10},
	}, 5)

	ch := make(chan *util.PeerMessage, 1)
	ch <- makePieceMsg(1, 0, data)
	close(ch)

	tor.WritePiece(ch, stats.NewStats(0))

	assert.Equal(t, data, readAt(t, tor.OsFiles[1], 0, len(data)))
}

func TestWritePiece_MultiFile_PieceSpansTwoFiles(t *testing.T) {
	// piecePoss=0, file a has 5 bytes, piece data is 8 bytes.
	// First 5 bytes go to file a, remaining 3 bytes go to file b at offset 0.
	dir := t.TempDir()
	data := []byte("12345678") // 8 bytes

	tor := makeMultiFileTorrent(t, dir, []bencode.TorrentFile{
		{Path: []string{"a.bin"}, Length: 5},
		{Path: []string{"b.bin"}, Length: 10},
	}, 15)

	ch := make(chan *util.PeerMessage, 1)
	ch <- makePieceMsg(0, 0, data)
	close(ch)

	tor.WritePiece(ch, stats.NewStats(0))

	assert.Equal(t, data[:5], readAt(t, tor.OsFiles[0], 0, 5), "first 5 bytes in file a")
	assert.Equal(t, data[5:], readAt(t, tor.OsFiles[1], 0, 3), "remaining 3 bytes in file b")
}
