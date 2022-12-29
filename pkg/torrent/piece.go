package torrent

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
)

type Piece struct {
	index int
	sha1  []byte
}

func newPiece(sha1 []byte, index int) Piece {
	return Piece{
		sha1:  sha1,
		index: index,
	}
}

func (p Piece) CheckHash(sha1 []byte) bool {
	return bytes.Compare(p.sha1, sha1) == 0
}

func (p Piece) Index() int {
	return p.index
}

func (p Piece) String() string {
	return fmt.Sprintf("[%d:%s]", p.index, base64.URLEncoding.EncodeToString(p.sha1))
}

func NewPieces(pieces []byte) ([]Piece, error) {
	if !checkPieceLen(pieces) {
		return nil, errors.New("invalid pieces len")
	}

	result := make([]Piece, piecesLen(pieces))
	index := 0
	for i, end := 0, len(pieces); i < end; {
		piece := newPiece(pieces[i:i+20], index)
		result[index] = piece
		i += 20
		index++
	}

	return result, nil
}

func checkPieceLen(pieces []byte) bool {
	return len(pieces)%20 == 0
}

func piecesLen(pieces []byte) int {
	return len(pieces) / 20
}
