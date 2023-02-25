package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"github.com/tevino/abool/v2"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bits-and-blooms/bitset"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/util"
)

const BlockLength uint = 64 * 1024

var log = zap.L()

var (
	BittorrentProto = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}
	clientIdPrefix  = [8]byte{'-', 'G', 'O', '0', '1', '0', '0', '-'}
)

type TorrentMetadata struct {
	Announce     string   `ben:"announce"`
	AnnounceList []string `ben:"announce-list"`
	UrlList      []string `ben:"url-list"`
	Info         struct {
		Files       []TorrentFile `ben:"files"`
		Length      int64         `ben:"length"`
		Name        string        `ben:"name"`
		PieceLength int           `ben:"piece length"`
		Pieces      string        `ben:"pieces"`
	} `ben:"info"`
	InfoDict     *bencode.DictElement `ben:"info"`
	Comment      string               `ben:"comment"`
	CreatedBy    string               `ben:"created by"`
	CreationDate int64                `ben:"creation date"`
}

func (torrent *Torrent) String() string {
	pieces := "["
	for _, piece := range torrent.Pieces {
		pieces = fmt.Sprintln(pieces, piece.Index(), ":", piece.String(), ",")
	}
	pieces = strings.TrimRight(pieces, ",")
	pieces = fmt.Sprintln(pieces, "]")
	return fmt.Sprint(fmt.Sprintln("Torrent {"),
		fmt.Sprintln("Pieces:", pieces),
		fmt.Sprintln("Name:", torrent.Name),
		fmt.Sprintln("Announce:", torrent.Metadata.Announce),
		fmt.Sprintln("Announce list:", torrent.Metadata.AnnounceList),
		fmt.Sprintln("Created by:", torrent.CreatedBy),
		fmt.Sprintln("Creation date:", torrent.CreationDate),
		fmt.Sprintln("Pieces num:", torrent.PiecesNum),
		fmt.Sprintln("Pieces len:", torrent.PieceLength),
		fmt.Sprintln("Comment:", torrent.Comment),
		fmt.Sprintln("Torrent files:", torrent.TorrentFiles),
		fmt.Sprint("}"))
}

// MarshalLogObject implements zapcore.ObjectMarshaler for logging
func (f *TorrentMetadata) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if f == nil {
		return nil
	}

	enc.AddString("announce", f.Announce)
	enc.AddReflected("announce-list", f.AnnounceList)
	enc.AddReflected("url-list", f.UrlList)
	enc.AddInt64("info.lenght", f.Info.Length)
	enc.AddInt("info.piece_length", f.Info.PieceLength)
	enc.AddString("info.name", f.Info.Name)
	enc.AddReflected("info.files", f.Info.Files)
	enc.AddString("comment", f.Comment)
	enc.AddString("created by", f.CreatedBy)
	enc.AddInt64("cration date", f.CreationDate)
	return nil
}

type Torrent struct {
	Trackers     util.StringSet
	Hash         []byte
	Length       int
	PieceLength  int
	Pieces       []Piece
	PiecesNum    int
	TorrentFiles []TorrentFile
	OsFiles      []*os.File
	Name         string
	CreationDate int64
	CreatedBy    string
	Comment      string
	IsDirectory  bool
	PeerId       []byte

	Metadata *TorrentMetadata

	numOfBlocks int

	requested   *bitset.BitSet
	requestedMu *sync.Mutex

	downloaded   *bitset.BitSet
	downloadedMu *sync.Mutex

	done   *abool.AtomicBool
	doneCh chan struct{}
}

type TorrentFile struct {
	Path   []string `ben:"path"`
	Length int      `ben:"length"`
}

func NewTorrent(meta *TorrentMetadata, downloadDir string) (*Torrent, error) {
	t := &Torrent{
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
		Metadata:     meta,
	}
	t.PeerId = createClientId()
	t.Name = meta.Info.Name
	t.CreatedBy = meta.CreatedBy
	t.CreationDate = meta.CreationDate
	t.Comment = meta.Comment
	t.PieceLength = meta.Info.PieceLength
	pieces, err := NewPieces([]byte(meta.Info.Pieces))
	if err != nil {
		return nil, err
	}
	t.Pieces = pieces
	t.requested = bitset.New(uint(t.PieceLength))
	t.downloaded = bitset.New(uint(t.PieceLength))
	t.numOfBlocks = t.PieceLength / int(BlockLength)

	info := meta.InfoDict.Raw()
	sha := sha1.New()
	sha.Write(info)
	t.Hash = sha.Sum(nil)

	trackers := meta.AnnounceList
	url_list := meta.UrlList
	announce := meta.Announce
	announceSet := util.NewStringSet()
	announceSet.Add(announce)
	for _, el := range trackers {
		announceSet.Add(el)
	}
	for _, el := range url_list {
		announceSet.Add(el)
	}
	t.Trackers = announceSet

	if meta.Info.Length != 0 {
		t.IsDirectory = false
		t.Length = int(meta.Info.Length)
	} else {
		t.IsDirectory = true
		files := meta.Info.Files

		var completeLength int = 0
		for _, file := range files {
			completeLength += file.Length
		}
		t.TorrentFiles = meta.Info.Files
		t.Length = completeLength
	}
	t.PiecesNum = int(math.Ceil(float64(t.Length) / float64(t.PieceLength)))

	if err := t.createTorrentFiles(downloadDir); err != nil {
		return nil, err
	}

	return t, nil
}

func (torrent Torrent) CreateHandshake() []byte {
	request := new(bytes.Buffer)
	// 19 - as number of letters in protocol type string
	binary.Write(request, binary.BigEndian, uint8(len(BittorrentProto)))
	binary.Write(request, binary.BigEndian, BittorrentProto)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, torrent.Hash)
	binary.Write(request, binary.BigEndian, torrent.PeerId)

	return request.Bytes()
}

// implemented BEP20
func createClientId() []byte {
	peerId := make([]byte, 20)
	copy(peerId, clientIdPrefix[:])

	// create remaining random bytes
	rand.Read(peerId[len(clientIdPrefix):])
	log.Debug("Created client id", zap.String("id", string(peerId)))
	return peerId
}

func (torrent *Torrent) SetDownloaded(pieceIndx uint) {
	torrent.downloaded.Set(pieceIndx)
}

func (torrent *Torrent) CreateNextRequestMessage(have *bitset.BitSet) (uint, bool) {
	indx, found := uint(0), false

	torrent.requestedMu.Lock()
	defer torrent.requestedMu.Unlock()
	for i, err := torrent.requested.NextClear(0); err; i, err = torrent.requested.NextClear(i) {
		if have.Test(i) {
			indx = i
			found = true
			torrent.requested.Set(i)
			break
		}
	}
	return indx, found
}

func (torrent *Torrent) Done() bool {
	torrent.downloadedMu.Lock()
	defer torrent.downloadedMu.Unlock()
	return torrent.downloaded.All()
}

func (torrent *Torrent) createTorrentFiles(root string) error {
	path := filepath.Join(root, torrent.Name)
	var filePaths []string
	if torrent.IsDirectory {
		if err := os.Mkdir(path, os.ModePerm); err != nil && os.IsNotExist(err) {
			return err
		}

		for _, tf := range torrent.TorrentFiles {
			filePaths = append(filePaths, filepath.Join(path, tf.Path[0]))
		}
	} else {
		filePaths = append(filePaths, path)
	}

	for _, path := range filePaths {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		torrent.OsFiles = append(torrent.OsFiles, f)
	}

	return nil
}

func (torrent *Torrent) WritePiece(piecesCh <-chan *util.PeerMessage) {
	writeFunc := func(msg *util.PeerMessage, piecePoss int) {
		file := torrent.OsFiles[0]
		file.WriteAt(msg.Payload[8:], int64(piecePoss))
	}

	if torrent.IsDirectory {
		writeFunc = func(msg *util.PeerMessage, piecePoss int) {

			torFiles := torrent.TorrentFiles
			for indx, torFile := range torFiles {
				if torFile.Length < piecePoss {
					piecePoss = piecePoss - torFile.Length
					continue
				} else {
					log.Debug("Writting to file ",
						zap.String("file", torFile.Path[0]),
						zap.Int("possition", piecePoss))

					pieceLen := len(msg.Payload[8:])
					unoccupiedLength := torFile.Length - piecePoss
					file := torrent.OsFiles[indx]
					if unoccupiedLength > pieceLen {
						file.WriteAt(msg.Payload[8:], int64(piecePoss))
					} else {
						file.WriteAt(msg.Payload[8:8+unoccupiedLength], int64(piecePoss))
						piecePoss += unoccupiedLength
						file = torrent.OsFiles[indx+1]

						file.WriteAt(msg.Payload[8+unoccupiedLength:], 0)
					}
					break
				}
			}
		}
	}

	for msg := range piecesCh {
		indx := binary.BigEndian.Uint32(msg.Payload[:4])
		offset := binary.BigEndian.Uint32(msg.Payload[4:8])
		piecePoss := int(indx)*torrent.PieceLength + int(offset)

		if (int(offset) + int(BlockLength)) == torrent.PieceLength {
			torrent.SetDownloaded(uint(indx))
		}

		writeFunc(msg, piecePoss)
	}
}

func (torrent *Torrent) BlockNum() int {
	return torrent.numOfBlocks
}

// Close torrent os files
func (torrent *Torrent) Close() (err error) {
	for _, f := range torrent.OsFiles {
		err = multierr.Append(err, f.Close())
	}
	return
}
