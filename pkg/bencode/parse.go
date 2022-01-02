package bencode

import (
	"errors"
	"strconv"
	"unicode"
)

var (
	ErrElementEnd   = errors.New("element not ended with 'e'")
	ErrColonMissing = errors.New("missing ':' in string element")
	ErrStringLength = errors.New("string length invalid")
)

var (
	stringNil = StringElement("")
	intNil    = IntElement(0)
)

func Parse(data string) ([]Bencode, error) {
	sc := NewScanner(data)
	return sc.Parse()
}

type scanner struct {
	start   int
	current int
	bencode string
}

func NewScanner(data string) *scanner {
	return &scanner{
		start:   0,
		current: 0,
		bencode: data,
	}
}

func (s *scanner) Parse() ([]Bencode, error) {
	elements := make([]Bencode, 0)
	for !s.IsFinished() {
		e, err := s.Next()
		if err != nil {
			return nil, err
		}
		elements = append(elements, e)
	}

	return elements, nil
}

func (s *scanner) Next() (Bencode, error) {
	switch s.advance() {
	case 'l':
		return s.readList()
	case 'd':
		return s.readDict()
	case 'i':
		return s.readInt()
	default:
		return s.readString()
	}
}

func (s *scanner) IsFinished() bool {
	return s.current >= len(s.bencode)
}

func (s *scanner) advance() rune {
	s.current++
	return rune(s.bencode[s.current-1])
}

func (s scanner) peek() rune {
	if s.IsFinished() {
		return rune(0) // return '\0' character
	}
	return rune(s.bencode[s.current])
}

func (s *scanner) match(r rune) bool {
	if s.peek() == r {
		s.advance()
		s.position()
		return true
	}

	return false
}

func (s scanner) read() string {
	return s.bencode[s.start:s.current]
}

func (s *scanner) position() {
	s.start = s.current
}

func (s scanner) isDigit() bool {
	return unicode.IsDigit(s.peek())
}

func (s *scanner) number() (int, error) {
	for s.isDigit() {
		s.advance()
	}
	value := s.read()
	s.position()
	return strconv.Atoi(value)
}

func (s *scanner) readInt() (IntElement, error) {
	s.position()
	n, err := s.number()
	if err != nil {
		return intNil, err
	}

	if !s.match('e') {
		return intNil, ErrElementEnd
	}
	return IntElement(n), nil
}

func (s *scanner) readString() (StringElement, error) {
	length, err := s.number()
	if err != nil {
		return stringNil, err
	}

	if !s.match(':') {
		return stringNil, ErrColonMissing
	}
	s.current += length
	// we need to check if we are trying to read beyond string length.
	if s.current > len(s.bencode) {
		return stringNil, ErrStringLength
	}

	strElement := StringElement(s.read())
	s.position()

	return strElement, nil
}

func (s *scanner) readList() (ListElement, error) {
	bencodeList := make([]Bencode, 0)
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

	return ListElement(bencodeList), nil
}

func (s *scanner) readDict() (DictElement, error) {
	dict := make(map[StringElement]Bencode)
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

	return DictElement(dict), nil
}
