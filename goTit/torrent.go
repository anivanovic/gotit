package main

import (
	"strconv"

	"crypto/sha1"

	"bytes"
	"encoding/binary"

	"sync"

	"github.com/anivanovic/goTit/bitset"
	"github.com/anivanovic/goTit/metainfo"
)

var BITTORENT_PROT = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}

type Torrent struct {
	Announce      string
	Announce_list []string
	Hash          []byte
	Length        int
	PieceLength   int
	Pieces        []byte
	Files         []TorrentFile
	Name          string
	CreationDate  int64
	CreatedBy     string
	Info          string
	Comment       string
	IsDirectory   bool

	bitfieldGuard sync.Mutex
	Bitset        bitset.BitSet
}

type TorrentFile struct {
	Path   string
	Length int
}

func NewTorrent(dictElement metainfo.DictElement) *Torrent {
	//TODO make bencode api simpler
	torrent := new(Torrent)
	torrent.Announce = dictElement.Value("announce").String()
	torrent.CreatedBy = dictElement.Value("created by").String()
	torrent.PieceLength, _ = strconv.Atoi(dictElement.Value("info.piece length").String())
	torrent.Name = dictElement.Value("info.name").String()
	torrent.Pieces = []byte(dictElement.Value("info.pieces").String())
	torrent.CreationDate, _ = strconv.ParseInt(dictElement.Value("creation date").String(), 10, 0)

	torrent.Info = dictElement.Value("info").Encode()

	sha := sha1.New()
	sha.Write([]byte(torrent.Info))
	hash := sha.Sum(nil)
	torrent.Hash = hash

	trackers := dictElement.Value("announce-list")
	trackersList, _ := trackers.(metainfo.ListElement)

	announceList := make([]string, 0)
	for _, elem := range trackersList.List {
		elemList, _ := elem.(metainfo.ListElement)
		announceList = append(announceList, elemList.List[0].String())
	}
	torrent.Announce_list = announceList

	if dictElement.Value("info.length") != nil {
		torrent.IsDirectory = false
		torrent.Length = dictElement.Value("info.length").(metainfo.IntElement).Value
	} else {
		torrent.IsDirectory = true
		files := dictElement.Value("info.files")
		filesList, _ := files.(metainfo.ListElement)

		torrentFiles := make([]TorrentFile, 0)
		var completeLength int = 0
		for _, file := range filesList.List {
			fileDict, _ := file.(metainfo.DictElement)
			length := fileDict.Value("length").(metainfo.IntElement)
			pathList, _ := fileDict.Value("path").(metainfo.ListElement)
			torrentFile := TorrentFile{Path: pathList.List[0].(metainfo.StringElement).Value,
				Length: length.Value}
			completeLength += torrentFile.Length

			torrentFiles = append(torrentFiles, torrentFile)
		}
		torrent.Length = completeLength
	}

	if comment := dictElement.Value("comment"); comment != nil {
		torrent.Comment = comment.(metainfo.StringElement).Value
	}

	return torrent
}

func (torrent Torrent) CreateHandshake(peerId []byte) []byte {
	request := new(bytes.Buffer)
	// 19 - as number of letters in protocol type string
	binary.Write(request, binary.BigEndian, uint8(len(BITTORENT_PROT)))
	binary.Write(request, binary.BigEndian, BITTORENT_PROT)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, torrent.Hash)
	binary.Write(request, binary.BigEndian, peerId)

	return request.Bytes()
}

func (torrent Torrent) SetDownloaded(pieceIndx int) {
	// bit in byte represents piece
	sliceIndex := pieceIndx / 8
	shift := uint32((9+pieceIndx)%8 - 1)
	bitmask := 128 // 0b10000000
	torrent.bitfieldGuard.Lock()
	bitmask = bitmask << shift
	torrent.bitfieldGuard.Unlock()
}
