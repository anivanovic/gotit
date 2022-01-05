package gotit

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"strconv"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/anivanovic/gotit/pkg/bitset"

	log "github.com/sirupsen/logrus"
)

var BITTORENT_PROT = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}
var CLIENT_ID = [8]byte{'-', 'G', 'O', '0', '1', '0', '0', '-'}

const blockLength uint32 = 16 * 1024

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
	Info         string
	Comment      string
	IsDirectory  bool
	PeerId       []byte

	downloaded, uploaded, left int

	pieceOffset int
	numOfBlocks int

	Bitset *bitset.BitSet
}

type TorrentFile struct {
	Path   string
	Length int
}

func NewTorrent(dictElement bencode.DictElement) *Torrent {
	//TODO make bencode api simpler
	torrent := new(Torrent)
	torrent.PeerId = createClientId()
	if dictElement.Value("created by") != nil {
		torrent.CreatedBy = dictElement.Value("created by").String()
	}
	torrent.PieceLength, _ = strconv.Atoi(dictElement.Value("info.piece length").String())
	torrent.Name = dictElement.Value("info.name").String()
	torrent.Pieces = []byte(dictElement.Value("info.pieces").String())
	torrent.CreationDate, _ = strconv.ParseInt(dictElement.Value("creation date").String(), 10, 0)
	torrent.Bitset = bitset.NewBitSet(torrent.PieceLength)

	torrent.Info = dictElement.Value("info").Encode()

	sha := sha1.New()
	sha.Write([]byte(torrent.Info))
	torrent.Hash = sha.Sum(nil)

	trackers := dictElement.Value("announce-list")
	trackersList, _ := trackers.(bencode.ListElement)
	announce := dictElement.Value("announce").String()
	announceSet := NewStringSet()
	announceSet.Add(announce)
	for _, elem := range trackersList {
		elemList, _ := elem.(bencode.ListElement)
		announceSet.Add(elemList[0].String())
	}
	torrent.Trackers = announceSet

	if dictElement.Value("info.length") != nil {
		torrent.IsDirectory = false
		length := dictElement.Value("info.length").(bencode.IntElement)
		torrent.Length = int(length)
	} else {
		torrent.IsDirectory = true
		files := dictElement.Value("info.files")
		filesList, _ := files.(bencode.ListElement)

		torrentFiles := make([]TorrentFile, 0)
		var completeLength int = 0
		for _, file := range filesList {
			fileDict, _ := file.(bencode.DictElement)
			length := fileDict.Value("length").(bencode.IntElement)
			pathList, _ := fileDict.Value("path").(bencode.ListElement)
			torrentFile := TorrentFile{Path: pathList[0].String(),
				Length: int(length)}
			completeLength += torrentFile.Length

			torrentFiles = append(torrentFiles, torrentFile)
		}
		torrent.TorrentFiles = torrentFiles
		torrent.Length = completeLength
		torrent.left = completeLength
	}
	torrent.numOfBlocks = torrent.PieceLength / int(blockLength)
	torrent.pieceOffset = -1

	if comment := dictElement.Value("comment"); comment != nil {
		torrent.Comment = comment.String()
	}
	torrent.PiecesNum = int(math.Ceil(float64(torrent.Length) / float64(torrent.PieceLength)))

	return torrent
}

func (torrent *Torrent) CreateHandshake() []byte {
	request := new(bytes.Buffer)
	// 19 - as number of letters in protocol type string
	binary.Write(request, binary.BigEndian, uint8(len(BITTORENT_PROT)))
	binary.Write(request, binary.BigEndian, BITTORENT_PROT)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, torrent.Hash)
	binary.Write(request, binary.BigEndian, torrent.PeerId)

	return request.Bytes()
}

// implemented BEP20
func createClientId() []byte {
	peerId := make([]byte, 20)
	copy(peerId, CLIENT_ID[:])

	// create remaining random bytes
	rand.Read(peerId[len(CLIENT_ID):])
	log.WithFields(log.Fields{
		"PEER_ID": string(peerId),
		"size":    len(peerId),
	}).Info("Created client id")
	return peerId
}

func (torrent *Torrent) SetDownloaded(pieceIndx int) {
	torrent.Bitset.Set(pieceIndx)
}

func (torrent *Torrent) nextDownladPiece() int {
	index := torrent.Bitset.FirstUnset(0)
	torrent.SetDownloaded(index)

	return index
}

func (torrent *Torrent) nextDownladBlock() int {
	if torrent.pieceOffset < torrent.numOfBlocks-1 {
		torrent.pieceOffset++
	} else {
		torrent.pieceOffset = 0
	}

	return torrent.pieceOffset
}

func (torrent *Torrent) CreateNextRequestMessage() []byte {
	beginOffset := torrent.nextDownladBlock() * int(blockLength)

	// TODO replace call to FirstUnet
	piece := torrent.Bitset.LastSet(0)
	if beginOffset == 0 {
		piece = torrent.nextDownladPiece()
	}

	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(13))
	binary.Write(message, binary.BigEndian, uint8(request))
	binary.Write(message, binary.BigEndian, uint32(piece))
	binary.Write(message, binary.BigEndian, uint32(beginOffset))
	binary.Write(message, binary.BigEndian, uint32(blockLength))
	log.WithFields(log.Fields{
		"piece":  piece,
		"offset": beginOffset,
		"length": blockLength,
	}).Debug("created piece request")

	return message.Bytes()
}
