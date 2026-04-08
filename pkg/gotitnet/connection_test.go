package gotitnet

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/anivanovic/gotit/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeConn creates a TimeoutConn backed by the client side of a net.Pipe.
// The returned server side is used to inject data the client will read.
func makeConn(t *testing.T) (*TimeoutConn, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return &TimeoutConn{c: client, timeout: time.Second}, server
}

// wirePeerMessage encodes a peer message as it appears on the wire:
// 4-byte big-endian length prefix followed by the body bytes.
func wirePeerMessage(body []byte) []byte {
	buf := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(body)))
	copy(buf[4:], body)
	return buf
}

// --- ReadPeerMessage ---------------------------------------------------------

func TestReadPeerMessage_ValidMessage(t *testing.T) {
	tc, srv := makeConn(t)

	body := []byte{byte(util.UnchokeMessageType)}
	go srv.Write(wirePeerMessage(body))

	got, err := tc.ReadPeerMessage()
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestReadPeerMessage_Keepalive(t *testing.T) {
	tc, srv := makeConn(t)

	// keepalive: 4-byte zero length, no body
	go srv.Write(make([]byte, 4))

	got, err := tc.ReadPeerMessage()
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestReadPeerMessage_MultiByteBody(t *testing.T) {
	tc, srv := makeConn(t)

	// piece message: type + 4 index + 4 offset + 8 data
	body := make([]byte, 17)
	body[0] = byte(util.PieceMessageType)
	binary.BigEndian.PutUint32(body[1:5], 3) // index
	binary.BigEndian.PutUint32(body[5:9], 0) // offset
	copy(body[9:], []byte("testdata"))
	go srv.Write(wirePeerMessage(body))

	got, err := tc.ReadPeerMessage()
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestReadPeerMessage_DeadlineError(t *testing.T) {
	deadlineErr := errors.New("set deadline failed")
	tc := &TimeoutConn{c: &mockConn{readDeadlineErr: deadlineErr}, timeout: time.Second}

	_, err := tc.ReadPeerMessage()
	assert.ErrorIs(t, err, deadlineErr)
}

// --- ReadPeerHandshake -------------------------------------------------------

func TestReadPeerHandshake_Valid(t *testing.T) {
	tc, srv := makeConn(t)

	payload := make([]byte, 68)
	payload[0] = 19
	copy(payload[1:20], "BitTorrent protocol")
	go srv.Write(payload)

	got, err := tc.ReadPeerHandshake()
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

func TestReadPeerHandshake_DeadlineError(t *testing.T) {
	deadlineErr := errors.New("set deadline failed")
	tc := &TimeoutConn{c: &mockConn{readDeadlineErr: deadlineErr}, timeout: time.Second}

	_, err := tc.ReadPeerHandshake()
	assert.ErrorIs(t, err, deadlineErr)
}

// --- WriteMsg ----------------------------------------------------------------

func TestWriteMsg_SendsOnWire(t *testing.T) {
	tc, srv := makeConn(t)

	msg := util.CreateUnchokeMessage()

	recv := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := srv.Read(buf)
		recv <- buf[:n]
	}()

	n, err := tc.WriteMsg(msg)
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	data := <-recv
	// wire format: 4-byte length + 1-byte type
	require.GreaterOrEqual(t, len(data), 5)
	assert.Equal(t, byte(util.UnchokeMessageType), data[4])
}

func TestWriteMsg_DeadlineError(t *testing.T) {
	deadlineErr := errors.New("set deadline failed")
	tc := &TimeoutConn{c: &mockConn{writeDeadlineErr: deadlineErr}, timeout: time.Second}

	_, err := tc.WriteMsg(util.CreateUnchokeMessage())
	assert.ErrorIs(t, err, deadlineErr)
}

// --- Write -------------------------------------------------------------------

func TestWrite_SendsBytes(t *testing.T) {
	tc, srv := makeConn(t)

	data := []byte("handshake-payload")
	recv := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := srv.Read(buf)
		recv <- buf[:n]
	}()

	n, err := tc.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, data, <-recv)
}

func TestWrite_DeadlineError(t *testing.T) {
	deadlineErr := errors.New("set deadline failed")
	tc := &TimeoutConn{c: &mockConn{writeDeadlineErr: deadlineErr}, timeout: time.Second}

	_, err := tc.Write([]byte("data"))
	assert.ErrorIs(t, err, deadlineErr)
}

// --- ReadAll -----------------------------------------------------------------

func TestReadAll_ReadsUntilEOF(t *testing.T) {
	tc, srv := makeConn(t)

	data := []byte("all the tracker data")
	go func() {
		srv.Write(data)
		srv.Close()
	}()

	got, err := tc.ReadAll()
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestReadAll_DeadlineError(t *testing.T) {
	deadlineErr := errors.New("set deadline failed")
	tc := &TimeoutConn{c: &mockConn{readDeadlineErr: deadlineErr}, timeout: time.Second}

	_, err := tc.ReadAll()
	assert.ErrorIs(t, err, deadlineErr)
}

// --- ReadUdpHandshake --------------------------------------------------------

func TestReadUdpHandshake_Valid(t *testing.T) {
	tc, srv := makeConn(t)

	payload := make([]byte, 16)
	for i := range payload {
		payload[i] = byte(i)
	}
	go srv.Write(payload)

	got, err := tc.ReadUdpHandshake()
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

func TestReadUdpHandshake_DoesNotCloseConnection(t *testing.T) {
	// Regression test: old code called c.c.Close() before reading.
	tc, srv := makeConn(t)

	payload := make([]byte, 16)
	go srv.Write(payload)

	_, err := tc.ReadUdpHandshake()
	require.NoError(t, err)

	// Connection must still be open after the call.
	assert.NoError(t, tc.Close())
}

func TestReadUdpHandshake_DeadlineError(t *testing.T) {
	deadlineErr := errors.New("set deadline failed")
	tc := &TimeoutConn{c: &mockConn{readDeadlineErr: deadlineErr}, timeout: time.Second}

	_, err := tc.ReadUdpHandshake()
	assert.ErrorIs(t, err, deadlineErr)
}

// --- mockConn ----------------------------------------------------------------

// mockConn is a minimal net.Conn that lets tests control deadline errors.
// Any Read/Write call on it will panic — deadline error tests never reach them.
type mockConn struct {
	net.Conn
	readDeadlineErr  error
	writeDeadlineErr error
}

func (m *mockConn) SetReadDeadline(_ time.Time) error  { return m.readDeadlineErr }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return m.writeDeadlineErr }
func (m *mockConn) Close() error                       { return nil }
