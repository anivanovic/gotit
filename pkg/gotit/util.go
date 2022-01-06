package gotit

import (
	"context"
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

func readConn(ctx context.Context, conn net.Conn) ([]byte, error) {
	conn.SetDeadline(time.Now().Add(time.Second * 2))
	data, err := ioutil.ReadAll(conn)
	if err != nil {
		log.WithError(err).Warn("error reading connection")
		return data, err
	}

	return data, nil
}
