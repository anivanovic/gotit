package util

import (
	"go.uber.org/zap"
)

var log = zap.L()

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

func SetLogger(l *zap.Logger) {
	log = l
}
