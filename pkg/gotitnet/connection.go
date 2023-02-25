package gotitnet

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"
)

const (
	PeerTimeout    = time.Second * 3
	TrackerTimeout = time.Second * 5
	DialTimeout    = time.Second * 5
)

type TimeoutConn struct {
	// Underlaying TCP/UDP connection.
	c net.Conn
	// Timeout used when readin or writing on
	// this connection.
	timeout time.Duration
}

func NewTimeoutConn(network, address string, timeout time.Duration) (*TimeoutConn, error) {
	conn, err := net.DialTimeout(network, address, DialTimeout)
	if err != nil {
		return nil, err
	}

	return &TimeoutConn{
			c:       conn,
			timeout: timeout,
		},
		nil
}

// ReadPeerMessage reads whole peer message from socket.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadPeerMessage(ctx context.Context) ([]byte, error) {
	size, err := c.readPeerMessageSize(ctx)
	if err != nil {
		return nil, err
	}

	return c.readExactly(ctx, size)
}

// readPeerMessageSize returns next peer message size by reading first 4
// bytes from socket.
func (c *TimeoutConn) readPeerMessageSize(ctx context.Context) (int, error) {
	buf, err := c.readExactly(ctx, 4)
	if err != nil {
		return 0, err
	}

	return int(binary.BigEndian.Uint32(buf)), nil
}

// ReadPeerHandshake reads peer handshake message from socket.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadPeerHandshake(ctx context.Context) ([]byte, error) {
	return c.readExactly(ctx, 68)
}

// Write writes data to socket.
// Write deadline is set to timeoutConn.timeout
func (c *TimeoutConn) Write(ctx context.Context, data []byte) (int, error) {
	c.setWriteDeadline()
	return c.c.Write(data)
}

// ReadAll reads from socket until error is thrown or EOF.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadAll(ctx context.Context) ([]byte, error) {
	c.setReadDeadline()
	return io.ReadAll(c.c)
}

// ReadUdpHandshake reads udp tracker handshake from socket.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadUdpHandshake(ctx context.Context) ([]byte, error) {
	return c.readExactly(ctx, 16)
}

func (c *TimeoutConn) Close() error {
	return c.c.Close()
}

func (c *TimeoutConn) readExactly(ctx context.Context, len int) ([]byte, error) {
	buf := make([]byte, len)
	c.setReadDeadline()
	if _, err := io.ReadFull(c.c, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

func (c *TimeoutConn) setReadDeadline() {
	c.c.SetReadDeadline(time.Now().Add(c.timeout))
}

func (c *TimeoutConn) setWriteDeadline() {
	c.c.SetWriteDeadline(time.Now().Add(c.timeout))
}
