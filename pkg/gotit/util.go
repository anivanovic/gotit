package gotit

import (
	"context"
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

func (s StringSet) AddAll(other map[string]struct{}) {
	for k := range other {
		s.Add(k)
	}
}

func (s StringSet) Contains(obj string) bool {
	_, ok := s[obj]
	return ok
}

func readConn(ctx context.Context, conn net.Conn) []byte {
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	for {
		conn.SetDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(tmp)
		if err != nil {
			log.WithError(err).Warnf("error reading connection %s", conn.RemoteAddr().String())
			break
		}
		response = append(response, tmp[:n]...)
	}

	return response
}
