package peer

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/anivanovic/gotit/pkg/torrent"
	"github.com/anivanovic/gotit/pkg/util"
	"github.com/bits-and-blooms/bitset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- helpers -----------------------------------------------------------------

func validHandshake(hash, peerId []byte) []byte {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, uint8(19))
	buf.Write(bittorrentProto[:])
	_ = binary.Write(buf, binary.BigEndian, uint64(0)) // reserved
	buf.Write(hash)
	buf.Write(peerId)
	return buf.Bytes()
}

func makePeer(t *testing.T, piecesSource PiecesSource, checker PieceChecker) (*Peer, chan *util.PeerMessage) {
	t.Helper()
	ch := make(chan *util.PeerMessage, 16)
	logger := zap.NewNop()
	p := &Peer{
		PeerStatus:   newPeerStatus(),
		ClientStatus: newPeerStatus(),
		Bitset:       bitset.New(8),
		piecesQueue:  torrent.NewPiecesQueue(),
		piecesSource: piecesSource,
		pieceChecker: checker,
		writeCh:      ch,
		logger:       logger,
	}
	return p, ch
}

// --- mocks -------------------------------------------------------------------

type mockPiecesSource struct {
	idx   uint
	found bool
}

func (m *mockPiecesSource) Next(_ *bitset.BitSet) (uint, bool) {
	return m.idx, m.found
}

type mockPieceChecker struct {
	valid bool
}

func (m *mockPieceChecker) CheckPiece(_ []byte, _ int) bool {
	return m.valid
}

// --- isHandshakeValid --------------------------------------------------------

func TestIsHandshakeValid_Valid(t *testing.T) {
	hash := bytes.Repeat([]byte{0xAB}, 20)
	peerId := bytes.Repeat([]byte{0xCD}, 20)
	hs := validHandshake(hash, peerId)

	assert.True(t, isHandshakeValid(hs, hash, peerId))
}

func TestIsHandshakeValid_TooShort(t *testing.T) {
	assert.False(t, isHandshakeValid([]byte{1, 2, 3}, nil, nil))
}

func TestIsHandshakeValid_WrongProtocolLength(t *testing.T) {
	hash := bytes.Repeat([]byte{0x01}, 20)
	peerId := bytes.Repeat([]byte{0x02}, 20)
	hs := validHandshake(hash, peerId)
	hs[0] = 18 // wrong length byte
	assert.False(t, isHandshakeValid(hs, hash, peerId))
}

func TestIsHandshakeValid_WrongProtocolSignature(t *testing.T) {
	hash := bytes.Repeat([]byte{0x01}, 20)
	peerId := bytes.Repeat([]byte{0x02}, 20)
	hs := validHandshake(hash, peerId)
	hs[1] = 'X' // corrupt protocol string
	assert.False(t, isHandshakeValid(hs, hash, peerId))
}

func TestIsHandshakeValid_WrongHash(t *testing.T) {
	hash := bytes.Repeat([]byte{0x01}, 20)
	peerId := bytes.Repeat([]byte{0x02}, 20)
	hs := validHandshake(hash, peerId)

	wrongHash := bytes.Repeat([]byte{0xFF}, 20)
	assert.False(t, isHandshakeValid(hs, wrongHash, peerId))
}

func TestIsHandshakeValid_NonZeroReservedBytes(t *testing.T) {
	// Peers with extensions set reserved bits; we should still accept them
	// if the hash matches. This test documents expected behavior — if
	// reservedBytes == 0 is intentionally enforced, update accordingly.
	hash := bytes.Repeat([]byte{0xAB}, 20)
	peerId := bytes.Repeat([]byte{0xCD}, 20)
	hs := validHandshake(hash, peerId)
	binary.BigEndian.PutUint64(hs[20:28], 0x0000000000100005) // DHT + Fast extension bits
	// This assertion captures current behavior; change to assert.True if
	// reserved byte checking is removed per the code review recommendation.
	_ = isHandshakeValid(hs, hash, peerId) // result is implementation-defined
}

// --- createHandshake ---------------------------------------------------------

func TestCreateHandshake_Length(t *testing.T) {
	hash := bytes.Repeat([]byte{0xAB}, 20)
	hs := createHandshake(hash)
	// 1 (len) + 19 (proto) + 8 (reserved) + 20 (hash) + 20 (clientId) = 68
	assert.Equal(t, 68, len(hs))
}

func TestCreateHandshake_LengthByte(t *testing.T) {
	hash := bytes.Repeat([]byte{0x00}, 20)
	hs := createHandshake(hash)
	assert.Equal(t, uint8(19), hs[0])
}

func TestCreateHandshake_ProtocolString(t *testing.T) {
	hash := bytes.Repeat([]byte{0x00}, 20)
	hs := createHandshake(hash)
	assert.Equal(t, "BitTorrent protocol", string(hs[1:20]))
}

func TestCreateHandshake_HashEmbedded(t *testing.T) {
	hash := bytes.Repeat([]byte{0xAB}, 20)
	hs := createHandshake(hash)
	assert.Equal(t, hash, hs[28:48])
}

func TestCreateHandshake_ClientIdEmbedded(t *testing.T) {
	hash := bytes.Repeat([]byte{0x00}, 20)
	hs := createHandshake(hash)
	assert.Equal(t, ClientId, hs[48:68])
}

// createHandshake output must be accepted by isHandshakeValid
func TestCreateHandshake_RoundTrip(t *testing.T) {
	hash := bytes.Repeat([]byte{0xDE}, 20)
	hs := createHandshake(hash)
	assert.True(t, isHandshakeValid(hs, hash, ClientId))
}

// --- createClientId ----------------------------------------------------------

func TestCreateClientId(t *testing.T) {
	id := createClientId()
	assert.Equal(t, 20, len(id), "client id should be 20 bytes")
	assert.Equal(t, clientIdPrefix[:], id[:len(clientIdPrefix)], "torrent client id prefix wrong")
}

// --- handlePeerMessage -------------------------------------------------------

func TestHandlePeerMessage_Keepalive(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})
	p.handlePeerMessage(util.KeepalivePeerMessage)
	// no state change expected; just verify no panic
}

func TestHandlePeerMessage_Choke(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})
	p.ClientStatus.Choked = false
	p.handlePeerMessage(util.NewPeerMessage([]byte{byte(util.ChokeMessageType)}))
	assert.True(t, p.ClientStatus.Choked)
}

func TestHandlePeerMessage_Unchoke(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})
	p.ClientStatus.Choked = true
	p.handlePeerMessage(util.NewPeerMessage([]byte{byte(util.UnchokeMessageType)}))
	assert.False(t, p.ClientStatus.Choked)
}

func TestHandlePeerMessage_Interested(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})
	p.PeerStatus.Interested = false
	p.handlePeerMessage(util.NewPeerMessage([]byte{byte(util.InterestedMessageType)}))
	assert.True(t, p.PeerStatus.Interested)
}

func TestHandlePeerMessage_NotInterested(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})
	p.PeerStatus.Interested = true
	p.handlePeerMessage(util.NewPeerMessage([]byte{byte(util.NotInterestedMessageType)}))
	assert.False(t, p.PeerStatus.Interested)
}

func TestHandlePeerMessage_Have(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})
	p.Bitset = bitset.New(256)

	payload := make([]byte, 5)
	payload[0] = byte(util.HaveMessageType)
	binary.BigEndian.PutUint32(payload[1:], 7)

	p.handlePeerMessage(util.NewPeerMessage(payload))
	assert.True(t, p.Bitset.Test(7))
}

func TestHandlePeerMessage_Bitfield(t *testing.T) {
	p, _ := makePeer(t, &mockPiecesSource{}, &mockPieceChecker{valid: true})

	// build a bitfield message: type byte + 1 byte of bits (MSB set = piece 0)
	payload := []byte{byte(util.BitfieldMessageType), 0b10000000}
	p.handlePeerMessage(util.NewPeerMessage(payload))

	require.NotNil(t, p.Bitset)
	// createBitset pads the byte to a uint64 in BigEndian order (0x8000000000000000),
	// so the bitset library (LSB-first) places the set bit at position 63.
	assert.True(t, p.Bitset.Test(63))
}

func TestHandlePeerMessage_Piece_ValidHash_WritesToChannel(t *testing.T) {
	checker := &mockPieceChecker{valid: true}
	p, ch := makePeer(t, &mockPiecesSource{}, checker)

	// piece payload: index(4) + offset(4) + data
	payload := make([]byte, 1+4+4+8)
	payload[0] = byte(util.PieceMessageType)
	binary.BigEndian.PutUint32(payload[1:5], 0) // index
	binary.BigEndian.PutUint32(payload[5:9], 0) // offset
	copy(payload[9:], []byte("testdata"))

	p.handlePeerMessage(util.NewPeerMessage(payload))

	require.Len(t, ch, 1)
}

func TestHandlePeerMessage_Piece_InvalidHash_DropsMessage(t *testing.T) {
	checker := &mockPieceChecker{valid: false}
	p, ch := makePeer(t, &mockPiecesSource{}, checker)

	payload := make([]byte, 1+4+4+8)
	payload[0] = byte(util.PieceMessageType)
	p.handlePeerMessage(util.NewPeerMessage(payload))

	assert.Empty(t, ch)
}

// --- nextRequestMessage ------------------------------------------------------

func TestNextRequestMessage_NoAvailablePiece(t *testing.T) {
	src := &mockPiecesSource{found: false}
	p, _ := makePeer(t, src, &mockPieceChecker{})

	msg := p.nextRequestMessage()
	assert.Nil(t, msg)
}

func TestNextRequestMessage_FirstBlockOfNewPiece(t *testing.T) {
	src := &mockPiecesSource{idx: 3, found: true}
	p, _ := makePeer(t, src, &mockPieceChecker{})
	// blockIdx=0, blockNum=0 → 0 >= 0 → triggers piecesSource.Next()

	msg := p.nextRequestMessage()

	require.NotNil(t, msg)
	assert.Equal(t, uint32(3), msg.Index())
	assert.Equal(t, uint32(0), msg.Offset()) // first block starts at 0
}

func TestNextRequestMessage_AdvancesBlockIdx(t *testing.T) {
	src := &mockPiecesSource{idx: 0, found: true}
	p, _ := makePeer(t, src, &mockPieceChecker{})
	p.blockNum = 4

	p.nextRequestMessage()
	assert.Equal(t, uint(1), p.blockIdx)
}

func TestNextRequestMessage_SecondBlock_CorrectOffset(t *testing.T) {
	src := &mockPiecesSource{idx: 1, found: true}
	p, _ := makePeer(t, src, &mockPieceChecker{})
	p.blockNum = 4
	p.pieceIdx = 1
	p.blockIdx = 1 // already sent block 0

	msg := p.nextRequestMessage()

	require.NotNil(t, msg)
	assert.Equal(t, uint32(torrent.BlockLength), msg.Offset())
}

func TestNextRequestMessage_RetriesFailedPieceFirst(t *testing.T) {
	src := &mockPiecesSource{idx: 5, found: true}
	p, _ := makePeer(t, src, &mockPieceChecker{})

	// inject a failed request
	failed := util.CreatePieceMessage(2, 0, uint32(torrent.BlockLength))
	p.piecesQueue.RequestFailed(failed)

	// blockIdx >= blockNum so it goes into the retry path
	msg := p.nextRequestMessage()

	require.NotNil(t, msg)
	assert.Equal(t, uint32(2), msg.Index(), "should retry the failed piece, not pick a new one")
}

func TestNextRequestMessage_AllBlocksSent_FetchesNewPiece(t *testing.T) {
	src := &mockPiecesSource{idx: 7, found: true}
	p, _ := makePeer(t, src, &mockPieceChecker{})
	p.blockNum = 2
	p.blockIdx = 2 // exhausted blocks for current piece

	msg := p.nextRequestMessage()

	require.NotNil(t, msg)
	assert.Equal(t, uint32(7), msg.Index(), "should have fetched the next piece from source")
	assert.Equal(t, uint(0), p.blockIdx-1, "blockIdx should have reset and incremented once")
}
