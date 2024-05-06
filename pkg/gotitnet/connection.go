package gotitnet

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/anivanovic/gotit/pkg/util"
)

const (
	PeerTimeout    = time.Second * 1
	TrackerTimeout = time.Millisecond * 500
	DialTimeout    = time.Second * 2
)

var before = time.Unix(1, 0)

type TimeoutConn struct {
	// Underlying TCP/UDP connection.
	c net.Conn
	// Timeout used when reading or writing on
	// this connection.
	timeout time.Duration
}

func NewTimeoutConn(network, address string, timeout time.Duration) (*TimeoutConn, error) {
	conn, err := net.DialTimeout(network, address, DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("timedConn (%s): %w", address, err)
	}

	return &TimeoutConn{
			c:       conn,
			timeout: timeout,
		},
		nil
}

// ReadPeerMessage reads whole peer message from socket.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadPeerMessage() ([]byte, error) {
	size, err := c.readPeerMessageSize()
	if err != nil {
		return nil, err
	}

	return c.readExactly(size)
}

// readPeerMessageSize returns next peer message size by reading first 4
// bytes from socket.
func (c *TimeoutConn) readPeerMessageSize() (int, error) {
	buf, err := c.readExactly(4)
	if err != nil {
		return 0, err
	}

	return int(binary.BigEndian.Uint32(buf)), nil
}

// ReadPeerHandshake reads peer handshake message from socket.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadPeerHandshake() ([]byte, error) {
	return c.readExactly(68)
}

// Write writes data to socket.
// Write deadline is set to timeoutConn.timeout
func (c *TimeoutConn) WriteMsg(msg *util.PeerMessage) (int, error) {
	c.setWriteDeadline()
	return msg.Send(c.c)
}

func (c *TimeoutConn) Write(data []byte) (int, error) {
	c.setWriteDeadline()
	return c.c.Write(data)
}

// ReadAll reads from socket until error is thrown or EOF.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadAll() ([]byte, error) {
	c.setReadDeadline()
	return io.ReadAll(c.c)
}

// ReadUdpHandshake reads udp tracker handshake from socket.
// Read deadline is set to timeoutConn.timeout
func (c *TimeoutConn) ReadUdpHandshake() ([]byte, error) {
	c.c.Close()
	return c.readExactly(16)
}

func (c *TimeoutConn) Close() error {
	return c.c.Close()
}

func (c *TimeoutConn) readExactly(len int) ([]byte, error) {
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
