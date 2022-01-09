package gotit

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"errors"

	"math/rand"

	"github.com/anivanovic/gotit/pkg/bitset"
	log "github.com/sirupsen/logrus"
)

const (
	choke         = iota // 0
	unchoke              // 1
	interested           // 2
	notInterested        // 3
	have                 // 4
	bitfield             // 5
	request              // 6
	piece                // 7
	cancel               // 8
)

const (
	peerTimeout = time.Second * 5
	readMax     = 1050
)

type Peer struct {
	Id           int
	Url          string
	Conn         net.Conn
	mng          *torrentManager
	Torrent      *Torrent
	Bitset       *bitset.BitSet
	PeerStatus   *PeerStatus
	ClientStatus *PeerStatus
	start        time.Time
	logger       *log.Entry
}

type PeerStatus struct {
	Choking    bool
	Interested bool
	Valid      bool
}

type PeerMessage struct {
	size    uint32
	code    uint8
	Payload []byte
}

var keepalivePeerMessage = &PeerMessage{
	size:    0,
	code:    99,
	Payload: nil,
}

func NewPeerMessage(data []byte) *PeerMessage {
	if len(data) == 0 { // keepalive message
		return keepalivePeerMessage
	}

	return &PeerMessage{size: uint32(len(data)), code: data[0], Payload: data[1:]}
}

func createNotInterestedMessage() []byte {
	return createSignalMessage(notInterested)
}

func createInterestedMessage() []byte {
	return createSignalMessage(interested)
}

func createChokeMessage() []byte {
	return createSignalMessage(choke)
}

func createUnchokeMessage() []byte {
	return createSignalMessage(unchoke)
}

func createSignalMessage(code int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(1))
	binary.Write(message, binary.BigEndian, uint8(code))

	return message.Bytes()
}

func createBitfieldMessage(peer *Peer) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(len(peer.Bitset.InternalSet)))
	binary.Write(message, binary.BigEndian, uint8(bitfield))
	binary.Write(message, binary.BigEndian, peer.Bitset.InternalSet)

	return message.Bytes()
}

func createHaveMessage(pieceIdx int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(have))
	binary.Write(message, binary.BigEndian, uint32(pieceIdx))

	return message.Bytes()
}

func createCancleMessage(pieceIdx int) []byte {
	message := new(bytes.Buffer)
	binary.Write(message, binary.BigEndian, uint32(5))
	binary.Write(message, binary.BigEndian, uint8(cancel))
	binary.Write(message, binary.BigEndian, uint32(pieceIdx))

	return message.Bytes()
}

func checkHandshake(handshake, hash, peerId []byte) bool {
	if len(handshake) < 68 {
		return false
	}

	ressCode := uint8(handshake[0])
	protocolSignature := string(handshake[1:20])
	reservedBytes := binary.BigEndian.Uint64(handshake[20:28])
	sentHash := handshake[28:48]
	sentPeerId := handshake[48:68]
	log.WithFields(log.Fields{
		"resCode":            ressCode,
		"protocol signature": protocolSignature,
		"hash":               sentHash,
		"peerId":             sentPeerId,
	}).Debug("Peer handshake message")

	return ressCode != 19 ||
		protocolSignature != string(BITTORENT_PROT[:]) ||
		reservedBytes != 0 ||
		!bytes.Equal(sentHash, hash) ||
		!bytes.Equal(sentPeerId, peerId)
}

func newPeerStatus() *PeerStatus {
	return &PeerStatus{true, false, true}
}

func writePiece(msg *PeerMessage, torrent *Torrent) {
	indx := binary.BigEndian.Uint32(msg.Payload[:4])
	offset := binary.BigEndian.Uint32(msg.Payload[4:8])
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

				log.Debug("Piece msg for writing")
				pieceLen := len(msg.Payload[8:])
				unoccupiedLength := torFile.Length - piecePoss
				file := torrent.OsFiles[indx]
				if unoccupiedLength > pieceLen {
					file.WriteAt(msg.Payload[8:], int64(piecePoss))
				} else {
					file.WriteAt(msg.Payload[8:8+unoccupiedLength], int64(piecePoss))
					piecePoss += unoccupiedLength
					file = torrent.OsFiles[indx+1]

					log.WithFields(log.Fields{
						"file":      file.Name(),
						"possition": piecePoss,
					}).Debug("Writting to file ")
					file.WriteAt(msg.Payload[8+unoccupiedLength:], 0)
				}
				file.Sync()
				break
			}
		}
	} else {
		files := torrent.OsFiles
		file := files[0]
		file.WriteAt(msg.Payload[8:], int64(piecePoss))
		file.Sync()
	}
}

func NewPeer(ip string, torrent *Torrent, mng *torrentManager) *Peer {
	logger := log.WithFields(log.Fields{
		"url": ip,
	})

	return &Peer{rand.Int(), ip, nil, mng, torrent, bitset.NewBitSet(torrent.PiecesNum),
		newPeerStatus(), newPeerStatus(), time.Now(), logger}
}

func (peer *Peer) connect() error {
	conn, err := net.DialTimeout("tcp", peer.Url, time.Second*2)
	if err != nil {
		return fmt.Errorf("peer connect failed: %w", err)
	}

	peer.start = time.Now()
	peer.Conn = conn
	return nil
}

func (peer *Peer) Announce() error {
	err := peer.connect()
	if err != nil {
		return err
	}

	peer.setWriteTimeout(peerTimeout)
	peer.Conn.Write(peer.Torrent.CreateHandshake())

	response, err := readHandshake(context.TODO(), peer.Conn)
	if err != nil {
		return err
	}

	if valid := checkHandshake(response, peer.Torrent.Hash, peer.Torrent.PeerId); !valid {
		return errors.New("peer handshake invalid")
	}

	peer.logger.Info("announce to peer successfull")
	return nil
}

// Intended to be run in separate goroutin. Communicates with remote peer
// and downloads torrent
func (peer *Peer) GoMessaging(ctx context.Context, wg *sync.WaitGroup) {
	sentPieceMsg := false
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// continue the loop
		}

		peer.checkKeepAlive()

		if peer.PeerStatus.Choking {
			peer.ClientStatus.Interested = true
			interestedM := createInterestedMessage()
			peer.logger.Debug("Sending interested message")

			_, err := peer.sendMessage(interestedM)
			if err != nil {
				return
			}
		}

		var requestMsg []byte
		if !peer.PeerStatus.Choking && !sentPieceMsg {
			requestMsg = peer.mng.NextRequest()
			sentPieceMsg = true
			_, err := peer.sendMessage(requestMsg)
			if err != nil {
				return
			}
		}

		response, err := readMessage(context.Background(), peer.Conn)
		if err != nil {
			if sentPieceMsg {
				peer.mng.RequestFailed(requestMsg)
			}
			return
		}

		peer.handlePeerMesssage(NewPeerMessage(response))
	}
}

func (peer *Peer) checkKeepAlive() {
	if time.Since(peer.start).Minutes() > 1.9 {
		peer.logger.Debug("Sending keep alive message")
		peer.start = time.Now()
		peer.sendMessage(make([]byte, 4)) // send 0
	}
}

func (peer *Peer) handlePeerMesssage(message *PeerMessage) {
	// if keepalive wait 2 minutes and try again
	if message.size == 0 {
		peer.logger.Debug("Peer sent keepalive")
		time.Sleep(time.Minute * 2)
		return
	}

	switch message.code {
	case bitfield:
		peer.logger.Debug("Peer sent bitfield message")
		peer.Bitset.InternalSet = message.Payload
	case have:
		peer.logger.Debug("Peer sent have message")
		indx := int(binary.BigEndian.Uint32(message.Payload))
		peer.Bitset.Set(indx)
	case interested:
		peer.logger.Debug("Peer sent interested message")
		peer.PeerStatus.Interested = true
		// return choke or unchoke
	case notInterested:
		peer.logger.Debug("Peer sent notInterested message")
		peer.PeerStatus.Interested = false
		// return choke
	case choke:
		peer.logger.Debug("Peer sent choke message")
		peer.PeerStatus.Choking = true
		time.Sleep(time.Second * 30)
	case unchoke:
		peer.logger.Debug("Peer sent unchoke message")
		peer.PeerStatus.Choking = false
	case request:
		peer.logger.Debug("Peer sent request message")
	case piece:
		peer.logger.Debug("Peer sent piece message")
		peer.mng.UpdateStatus(uint64(peer.mng.torrent.PieceLength), 0)
		writePiece(message, peer.mng.torrent)
	case cancel:
		peer.logger.Info("Peer sent cancle message")
	default:
		peer.logger.Infof("Peer sent wrong code %d", message.code)
	}
}

func (peer *Peer) sendMessage(message []byte) (int, error) {
	peer.Conn.SetWriteDeadline(time.Now().Add(peerTimeout))
	n, err := peer.Conn.Write(message)
	peer.logger.WithFields(log.Fields{
		"written": n,
		"error":   err,
	}).Debug("sendMessage to peer")
	return n, err
}

func (peer *Peer) sendHave(payload []byte) {
	indx := binary.BigEndian.Uint32(payload[:4])
	peer.sendMessage(createHaveMessage(int(indx)))
}

func (p *Peer) setWriteTimeout(dur time.Duration) {
	p.Conn.SetWriteDeadline(time.Now().Add(dur))
}

func (p *Peer) setReadTimeout(dur time.Duration) {
	p.Conn.SetReadDeadline(time.Now().Add(dur))
}
