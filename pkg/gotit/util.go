package gotit

import (
	"context"
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

type StringSet map[string]struct{}

func NewStringSet() StringSet {
	return make(map[string]struct{})
}

func (s StringSet) Add(obj string) bool {
	if _, ok := s[obj]; !ok {
		s[obj] = struct{}{}
		return true
	}

	return false
}

func (s StringSet) AddAll(other StringSet) {
	for k := range other {
		s.Add(k)
	}
}

func (s StringSet) Contains(obj string) bool {
	_, ok := s[obj]
	return ok
}

// read peer message size from conn
func readSize(ctx context.Context, conn net.Conn) (int, error) {
	buf := make([]byte, 4)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return -1, err
	}

	return int(binary.BigEndian.Uint32(buf)), nil
}

// readMessage reads size prefixed messages from peer in Bittorent protocol
func readMessage(ctx context.Context, conn net.Conn) ([]byte, error) {
	size, err := readSize(ctx, conn)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, size)
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		log.WithError(err).Warn("error reading connection")
		return nil, err
	}

	return buf, nil
}

// readHandshake expects to read handshake from underlaying connection.
func readHandshake(ctx context.Context, conn net.Conn) ([]byte, error) {
	buf := make([]byte, 68)
	_, err := io.ReadFull(conn, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func readConn(ctx context.Context, conn net.Conn) ([]byte, error) {
	conn.SetDeadline(time.Now().Add(time.Second * 2))
	data, err := ioutil.ReadAll(conn)
	if err != nil {
		log.WithError(err).Warn("error reading connection")
		return data, err
	}

	return data, nil
}
