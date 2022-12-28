package bencode

import (
	"errors"
	"unsafe"
)

var (
	ErrElementEnd   = errors.New("element not ended with 'e'")
	ErrColonMissing = errors.New("missing ':' in string element")
	ErrStringLength = errors.New("string length invalid")
)

func Parse(data []byte) (Bencode, error) {
	sc := newScanner(data)
	return sc.Parse()
}

type scanner struct {
	start   int
	current int
	bencode []byte
}

func newScanner(data []byte) *scanner {
	return &scanner{
		start:   0,
		current: 0,
		bencode: data,
	}
}

func (s *scanner) Parse() (Bencode, error) {
	if s.IsFinished() {
		return nil, errors.New("bencode: no data")
	}
	return s.Next()
}

func (s *scanner) Next() (Bencode, error) {
	switch s.peek() {
	case 'l':
		return s.readList()
	case 'd':
		return s.readDict()
	case 'i':
		return s.readInt()
	default:
		v, err := s.readString()
		return StringElement(v), err
	}
}

func (s *scanner) IsFinished() bool {
	return s.current >= len(s.bencode)
}

func (s *scanner) advance() rune {
	s.current++
	return rune(s.bencode[s.current-1])
}

func (s scanner) peek() byte {
	if s.IsFinished() {
		return 0 // return '\0' character
	}
	return s.bencode[s.current]
}

func (s *scanner) match(r byte) bool {
	if s.peek() == r {
		s.advance()
		s.position()
		return true
	}

	return false
}

func (s scanner) read() []byte {
	return s.bencode[s.start:s.current]
}

func (s *scanner) position() {
	s.start = s.current
}

func (s scanner) isDigit() bool {
	return s.peek() >= '0' && s.peek() <= '9'
}

func (s *scanner) number() (int, error) {
	n := 0
	for s.isDigit() {
		d := int(s.peek() - '0')
		n += d
		n *= 10
		s.advance()
	}
	n /= 10

	s.position()
	return n, nil
}

func (s *scanner) readInt() (IntElement, error) {
	s.advance()
	s.position()

	n, err := s.number()
	if err != nil {
		return 0, err
	}

	if !s.match('e') {
		return 0, ErrElementEnd
	}
	return IntElement(n), nil
}

func (s *scanner) readString() (string, error) {
	length, err := s.number()
	if err != nil {
		return "", err
	}

	if !s.match(':') {
		return "", ErrColonMissing
	}
	s.current += length
	// we need to check if we are trying to read beyond string length.
	if s.current > len(s.bencode) {
		return "", ErrStringLength
	}

	strElement := b2s(s.read())
	s.position()

	return strElement, nil
}

func (s *scanner) readList() (*ListElement, error) {
	bencodeList := make([]Bencode, 0)
	start := s.start
	s.advance()
	s.position()

	for s.peek() != 'e' && !s.IsFinished() {
		s.position()
		element, err := s.Next()
		if err != nil {
			return nil, err
		}
		bencodeList = append(bencodeList, element)
	}
	if !s.match('e') {
		return nil, ErrElementEnd
	}
	end := s.start
	raw := s.bencode[start:end]

	return &ListElement{Value: bencodeList, raw: raw}, nil
}

func (s *scanner) readDict() (*DictElement, error) {
	dict := make(map[string]Bencode)
	start := s.start
	s.advance()
	s.position()

	for s.peek() != 'e' && !s.IsFinished() {
		k, err := s.readString()
		if err != nil {
			return nil, err
		}
		v, err := s.Next()
		if err != nil {
			return nil, err
		}

		dict[k] = v
	}
	if !s.match('e') {
		return nil, ErrElementEnd
	}
	end := s.start
	raw := s.bencode[start:end]

	return &DictElement{value: dict, raw: raw}, nil
}

func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
