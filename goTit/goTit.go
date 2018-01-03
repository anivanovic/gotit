package main

import (
	"encoding/binary"
	"io/ioutil"
	"math/rand"

	"net"
	"net/url"
	"time"

	"os"

	"math"

	"github.com/anivanovic/goTit/metainfo"
	log "github.com/sirupsen/logrus"
)

const (
	letterBytes           = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	listenPort     uint16 = 8999
	DownloadFolder        = "C:/Users/Antonije/Downloads/"
)

// set up logger
func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}

func CheckError(err error) {
	if err != nil {
		log.Warnf("%T %+v", err, err)
	}
}

func randStringBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return b
}

func readConn(conn net.Conn) []byte {
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	for {
		conn.SetDeadline(time.Now().Add(timeout))
		n, err := conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		log.WithField("read data", n).Info("Read data from connection")
		response = append(response, tmp[:n]...)
	}

	return response
}

func readHandshake(conn net.Conn) []byte {
	response := make([]byte, 0, 68)

	conn.SetDeadline(time.Now().Add(timeout))
	n, err := conn.Read(response)
	if err != nil {
		CheckError(err)
		return response
	}
	log.WithField("read data", n).Info("Read data from peer handshake")

	return response
}

func main() {
	//torrentContent, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/Alien- Covenant (2017) [720p] [YTS.AG].torrent")
	torrentContent, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/viking.torrent")
	torrentString := string(torrentContent)
	_, benDict := metainfo.Parse(torrentString)
	log.Info("Parsed torrent file")
	log.Debug(benDict.String())

	transactionId := uint32(12345612)
	peerId := randStringBytes(20)

	torrent := NewTorrent(*benDict)

	announceList := torrent.Announce_list
	announceList = append(announceList, torrent.Announce)
	ips := make(map[string]bool)

	for _, trackerUrl := range announceList {
		u, err := url.Parse(trackerUrl)
		CheckError(err)
		if u.Scheme == "udp" {
			tracker_ips := announce(u, transactionId, torrent.Hash, peerId)
			for k, v := range *tracker_ips {
				ips[k] = v
			}
		}
	}

	log.WithField("size", len(ips)).Info("Peers in pool")
	createTorrentFiles(torrent)

	mng := NewMng(torrent)
	pieceCh := make(chan *peerMessage, 1000)
	waitCh := make(chan bool)
	go writePiece(pieceCh, torrent)
	//func() {
	indx := 0
	for ip, _ := range ips {
		if indx < 500 {
			go func(ip string, torrent *Torrent) {
				peer := NewPeer(ip, torrent, mng, pieceCh)
				err := peer.Announce(peerId)
				CheckError(err)
				if err == nil {
					peer.GoMessaging()
				}
			}(ip, torrent)
			indx++
		}
	}
	//}()

	<-waitCh
}

func createTorrentFiles(torrent *Torrent) {
	//user, err := user.Current()
	//CheckError(err)

	downloadsDir := "D:/Downloads/"
	torrent.OsFiles = make([]*os.File, 0)

	torrentDirPath := downloadsDir + torrent.Name
	if torrent.IsDirectory {
		err := os.Mkdir(torrentDirPath, os.ModeDir)
		CheckError(err)
		for _, torrentFile := range torrent.TorrentFiles {
			file, err := os.Create(torrentDirPath + "/" + torrentFile.Path)
			CheckError(err)
			prepopulateFile(file, torrentFile.Length)
			torrent.OsFiles = append(torrent.OsFiles, file)
		}
	} else {
		torrentFile, err := os.Create(torrentDirPath)
		CheckError(err)
		prepopulateFile(torrentFile, torrent.Length)
		torrent.OsFiles = append(torrent.OsFiles, torrentFile)
	}
}

func prepopulateFile(file *os.File, length int) {
	fileSize := math.Ceil(float64(length) / 8.0)
	zeroes := make([]byte, int(fileSize))
	file.Write(zeroes)
}

func writePiece(pieceCh <-chan *peerMessage, torrent *Torrent) {
	for true {
		pieceMsg := <-pieceCh
		indx := binary.BigEndian.Uint32(pieceMsg.payload[:4])
		offset := binary.BigEndian.Uint32(pieceMsg.payload[4:8])
		log.WithFields(log.Fields{
			"index":  indx,
			"offset": offset,
		}).Debug("Received piece message for writing to file")
		piecePoss := (int(indx)*torrent.PieceLength + int(offset))

		if torrent.IsDirectory {
			torFiles := torrent.TorrentFiles
			for indx, torFile := range torFiles {
				if torFile.Length < piecePoss {
					piecePoss = piecePoss - torFile.Length
					continue
				} else {
					log.WithFields(log.Fields{
						"file":      torFile.Path,
						"possition": piecePoss,
					}).Debug("Writting to file ")

					log.WithField("size", pieceMsg.size).Debug("Piece msg for writing")
					pieceLen := len(pieceMsg.payload[8:])
					unoccupiedLength := torFile.Length - piecePoss
					file := torrent.OsFiles[indx]
					if unoccupiedLength > pieceLen {
						file.WriteAt(pieceMsg.payload[8:], int64(piecePoss))
					} else {
						file.WriteAt(pieceMsg.payload[8:8+unoccupiedLength], int64(piecePoss))
						piecePoss += unoccupiedLength
						file = torrent.OsFiles[indx+1]

						log.WithFields(log.Fields{
							"file":      file.Name(),
							"possition": piecePoss,
						}).Debug("Writting to file ")
						file.WriteAt(pieceMsg.payload[8+unoccupiedLength:], 0)
					}
					file.Sync()
					break
				}
			}
		} else {
			files := torrent.OsFiles
			file := files[0]
			file.WriteAt(pieceMsg.payload[8:], int64(piecePoss))
			file.Sync()
		}
	}
}

func announce(u *url.URL, transactionId uint32, hash []byte, peerId []byte) *map[string]bool {
	tracker := Tracker(u)
	defer tracker.Close()
	connId := tracker.Handshake(transactionId)
	ips := tracker.Announce(connId, hash, transactionId, peerId)
	return ips
}
