package gotit

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/bits-and-blooms/bitset"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const blockLength uint = 64 * 1024

var (
	bittorentProto = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}
	clientIdPrefix = [8]byte{'-', 'G', 'O', '0', '1', '0', '0', '-'}
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
	Trackers     StringSet
	Hash         []byte
	Length       int
	PieceLength  int
	Pieces       []byte
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
}

type TorrentFile struct {
	Path   []string `ben:"path"`
	Length int      `ben:"length"`
}

func NewTorrent(meta *TorrentMetadata, downloadDir string) (*Torrent, error) {
	torrent := &Torrent{
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
		Metadata:     meta,
	}
	torrent.PeerId = createClientId()
	torrent.Name = meta.Info.Name
	torrent.CreatedBy = meta.CreatedBy
	torrent.CreationDate = meta.CreationDate
	torrent.Comment = meta.Comment
	torrent.PieceLength = meta.Info.PieceLength
	torrent.Pieces = []byte(meta.Info.Pieces)
	torrent.PiecesNum = int(math.Ceil(float64(torrent.Length) / float64(torrent.PieceLength)))
	torrent.requested = bitset.New(uint(torrent.PieceLength))
	torrent.downloaded = bitset.New(uint(torrent.PieceLength))
	torrent.numOfBlocks = torrent.PieceLength / int(blockLength)

	info := meta.InfoDict.Raw()
	sha := sha1.New()
	sha.Write(info)
	torrent.Hash = sha.Sum(nil)

	trackers := meta.AnnounceList
	url_list := meta.UrlList
	announce := meta.Announce
	announceSet := NewStringSet()
	announceSet.Add(announce)
	for _, el := range trackers {
		announceSet.Add(el)
	}
	for _, el := range url_list {
		announceSet.Add(el)
	}
	torrent.Trackers = announceSet

	if meta.Info.Length != 0 {
		torrent.IsDirectory = false
		torrent.Length = int(meta.Info.Length)
	} else {
		torrent.IsDirectory = true
		files := meta.Info.Files

		var completeLength int = 0
		for _, file := range files {
			completeLength += file.Length
		}
		torrent.TorrentFiles = meta.Info.Files
		torrent.Length = completeLength
	}

	if err := torrent.createTorrentFiles(downloadDir); err != nil {
		return nil, err
	}

	return torrent, nil
}

func (torrent Torrent) CreateHandshake() []byte {
	request := new(bytes.Buffer)
	// 19 - as number of letters in protocol type string
	binary.Write(request, binary.BigEndian, uint8(len(bittorentProto)))
	binary.Write(request, binary.BigEndian, bittorentProto)
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

func (torrent *Torrent) writePiece(piecesCh <-chan *PeerMessage) {
	writeFunc := func(msg *PeerMessage, piecePoss int) {
		file := torrent.OsFiles[0]
		file.WriteAt(msg.Payload[8:], int64(piecePoss))
	}

	if torrent.IsDirectory {
		writeFunc = func(msg *PeerMessage, piecePoss int) {

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

		if (int(offset) + int(blockLength)) == torrent.PieceLength {
			torrent.SetDownloaded(uint(indx))
		}

		writeFunc(msg, piecePoss)
	}
}

// Close torrent os files
func (torrent *Torrent) Close() (err error) {
	for _, f := range torrent.OsFiles {
		err = multierr.Append(err, f.Close())
	}
	return
}
