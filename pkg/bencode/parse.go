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

func Parse(data string) ([]Bencode, error) {
	sc := NewScanner(data)
	return sc.parse()
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

func (s *scanner) isAtEnd() bool {
	return s.current >= len(s.bencode)
}

func (s *scanner) advance() rune {
	s.current++
	return rune(s.bencode[s.current-1])
}

func (s scanner) peek() rune {
	if s.isAtEnd() {
		return rune(0) // return '\0' character
	}
	return rune(s.bencode[s.current])
}

func (s *scanner) match(r rune) bool {
	if s.peek() == r {
		s.advance()
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

func (s *scanner) parse() ([]Bencode, error) {
	elements := make([]Bencode, 0)
	for !s.isAtEnd() {
		s.position()
		e, err := s.next()
		if err != nil {
			return nil, err
		}
		elements = append(elements, e)
	}

	return elements, nil
}

func (s *scanner) next() (Bencode, error) {
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

func (s *scanner) readInt() (*IntElement, error) {
	s.position()
	n, err := s.number()
	if err != nil {
		return nil, err
	}
	i := IntElement{Value: n}

	if !s.match('e') {
		return nil, ErrElementEnd
	}
	return &i, nil
}

func (s *scanner) readString() (*StringElement, error) {
	len, err := s.number()
	if err != nil {
		return nil, err
	}

	if !s.match(':') {
		return nil, ErrColonMissing
	}

	s.position()
	s.current += len
	// if s.isAtEnd() {
	// 	return nil, ErrStringLength
	// }

	value := s.read()
	strElement := StringElement{Value: value}

	return &strElement, nil
}

func (s *scanner) readList() (*ListElement, error) {
	bencodeList := make([]Bencode, 0)
	for s.peek() != 'e' {
		s.position()
		element, err := s.next()
		if err != nil {
			return nil, err
		}
		bencodeList = append(bencodeList, element)
	}
	if !s.match('e') {
		return nil, ErrElementEnd
	}

	return &ListElement{List: bencodeList}, nil
}

func (s *scanner) readDict() (*DictElement, error) {
	dict := make(map[StringElement]Bencode)
	for s.peek() != 'e' {
		s.position()
		k, err := s.readString()
		if err != nil {
			return nil, err
		}
		s.position()
		v, err := s.next()
		if err != nil {
			return nil, err
		}

		dict[*k] = v
	}
	if !s.match('e') {
		return nil, ErrElementEnd
	}

	return &DictElement{Dict: dict}, nil
}
