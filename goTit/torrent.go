package main

type Torrent struct {
	Announce string
	Announce_list []string
	Hash []byte
	Length int64
	PieceLength int
	Pieces []byte
	Files []TorrentFile
	Name string
	CreationDate int64
	CreatedBy string
}

type TorrentFile struct {
	Path string
	Length int64
}
