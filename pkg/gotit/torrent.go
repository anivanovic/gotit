package gotit

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"strconv"
	"sync"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/bits-and-blooms/bitset"
	"go.uber.org/zap"
)

const blockLength uint = 16 * 1024

var (
	bittorentProto = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}
	clientIdPrefix = [8]byte{'-', 'G', 'O', '0', '1', '0', '0', '-'}
)

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

	numOfBlocks int

	requested   *bitset.BitSet
	requestedMu *sync.Mutex

	downloaded   *bitset.BitSet
	downloadedMu *sync.Mutex
}

type TorrentFile struct {
	Path   string
	Length int
}

func NewTorrent(dictElement bencode.DictElement) *Torrent {
	//TODO make bencode api simpler
	torrent := &Torrent{
		requestedMu:  &sync.Mutex{},
		downloadedMu: &sync.Mutex{},
	}
	torrent.PeerId = createClientId()
	if dictElement.Value("created by") != nil {
		torrent.CreatedBy = dictElement.Value("created by").String()
	}
	torrent.PieceLength, _ = strconv.Atoi(dictElement.Value("info.piece length").String())
	torrent.Name = dictElement.Value("info.name").String()
	torrent.Pieces = []byte(dictElement.Value("info.pieces").String())
	torrent.CreationDate, _ = strconv.ParseInt(dictElement.Value("creation date").String(), 10, 0)
	torrent.requested = bitset.New(uint(torrent.PieceLength))
	torrent.downloaded = bitset.New(uint(torrent.PieceLength))

	torrent.Info = dictElement.Value("info").Encode()

	sha := sha1.New()
	sha.Write([]byte(torrent.Info))
	torrent.Hash = sha.Sum(nil)

	trackers := dictElement.Value("announce-list")
	trackersList, _ := trackers.(bencode.ListElement)
	// TODO merge reading of announce and ulr list
	url_list, _ := dictElement.Value("url-list").(bencode.ListElement)
	// announce := dictElement.Value("announce").String()
	announceSet := NewStringSet()
	// announceSet.Add(announce)
	for _, elem := range trackersList {
		elemList, _ := elem.(bencode.ListElement)
		announceSet.Add(elemList[0].String())
	}
	for _, elem := range url_list {
		announceSet.Add(elem.String())
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
	}
	torrent.numOfBlocks = torrent.PieceLength / int(blockLength)

	if comment := dictElement.Value("comment"); comment != nil {
		torrent.Comment = comment.String()
	}
	torrent.PiecesNum = int(math.Ceil(float64(torrent.Length) / float64(torrent.PieceLength)))

	return torrent
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
	torrent.downloadedMu.Lock()
	defer torrent.downloadedMu.Unlock()
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
