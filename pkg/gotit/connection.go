package gotit

import (
	"context"
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"
	"time"

	"go.uber.org/zap"
)

const (
	peerTimeout = time.Second * 5
)

type peerConn struct {
	c net.Conn
}

// read peer message size from conn
func (conn peerConn) readSize(ctx context.Context) (int, error) {
	buf := make([]byte, 4)
	conn.c.SetReadDeadline(time.Now().Add(peerTimeout))
	_, err := io.ReadFull(conn.c, buf)
	if err != nil {
		return -1, err
	}

	return int(binary.BigEndian.Uint32(buf)), nil
}

// readMessage reads size prefixed messages from peer in Bittorent protocol
func (conn peerConn) readMessage(ctx context.Context) ([]byte, error) {
	size, err := conn.readSize(ctx)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, size)
	conn.c.SetReadDeadline(time.Now().Add(peerTimeout))
	_, err = io.ReadFull(conn.c, buf)
	if err != nil {
		log.Warn("error reading connection", zap.Error(err))
		return nil, err
	}

	return buf, nil
}

// readHandshake expects to read handshake from underlaying connection.
func (conn peerConn) readHandshake(ctx context.Context) ([]byte, error) {
	conn.c.SetReadDeadline(time.Now().Add(peerTimeout))
	buf := make([]byte, 68)
	_, err := io.ReadFull(conn.c, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (conn peerConn) write(ctx context.Context, data []byte) (int, error) {
	conn.c.SetWriteDeadline(time.Now().Add(peerTimeout))
	return conn.c.Write(data)
}

func readConn(ctx context.Context, conn net.Conn) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(time.Second * 2))
	data, err := ioutil.ReadAll(conn)
	if err != nil {
		log.Warn("error reading connection", zap.Error(err))
		return data, err
	}

	return data, nil
}
