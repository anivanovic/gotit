package gotit

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"

	"syscall"

	"io"

	"errors"

	"strconv"

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
	peerTimeout = time.Second * 10
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
	PieceCh      chan<- *PeerMessage
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

func NewPeerMessage(data []byte) *PeerMessage {
	var message PeerMessage
	if len(data) == 0 { // keepalive message
		message = PeerMessage{size: 0, code: 99, Payload: nil}
	} else {
		message = PeerMessage{size: uint32(len(data)), code: data[0], Payload: data[1:]}
	}
	return &message
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
		protocolSignature != string(BITTORENT_PROT[:len(BITTORENT_PROT)]) ||
		reservedBytes != 0 ||
		bytes.Compare(sentHash, hash) != 0 ||
		bytes.Compare(sentPeerId, peerId) != 0
}

func newPeerStatus() *PeerStatus {
	return &PeerStatus{true, false, true}
}

func NewPeer(ip string, torrent *Torrent, mng *torrentManager, ch chan<- *PeerMessage) *Peer {
	logger := log.WithFields(log.Fields{
		"url": ip,
	})

	return &Peer{rand.Int(), ip, nil, mng, torrent, bitset.NewBitSet(torrent.PiecesNum),
		newPeerStatus(), newPeerStatus(), ch, time.Now(), logger}
}

// Intended to be run in separate goroutin. Communicates with remote peer
// and downloads torrent
func (peer *Peer) GoMessaging() {
	sentPieceMsg := false
	for {
		peer.checkKeepAlive()

		if peer.PeerStatus.Choking {
			peer.ClientStatus.Interested = true
			interestedM := createInterestedMessage()
			peer.logger.Info("Sending interested message")

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

		// read message from peer
		response, err := readPeerConn(peer)
		if err != nil {
			if sentPieceMsg {
				peer.mng.RequestFailed(requestMsg)
			}
			return
		}

		peerMessages := readResponse(response)
		for _, message := range peerMessages {
			if sentPieceMsg {
				sentPieceMsg = false
			}
			peer.handlePeerMesssage(&message)
		}
	}
}

func (peer *Peer) checkKeepAlive() {
	elapsed := time.Since(peer.start)
	if elapsed.Minutes() > 1.9 {
		peer.logger.Info("Sending keep alive message")
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
		peer.PieceCh <- message
		//peer.sendHave(message.payload)
	case cancel:
		peer.logger.Info("Peer received cancle message")
	}
}

func (peer *Peer) sendHave(payload []byte) {
	indx := binary.BigEndian.Uint32(payload[:4])
	peer.sendMessage(createHaveMessage(int(indx)))
}

//////////////////////
// PEER IO METHODES //
//////////////////////
func (peer *Peer) sendMessage(message []byte) (int, error) {
	peer.Conn.SetWriteDeadline(time.Now().Add(peerTimeout))
	n, err := peer.Conn.Write(message)
	peer.logger.WithFields(log.Fields{
		"written": n,
		"error":   err,
	}).Debug("sendMessage to peer")
	return n, err
}

func readResponse(response []byte) []PeerMessage {
	read := len(response)
	currPossition := 0

	messages := make([]PeerMessage, 0)
	for currPossition < read {
		size := int(binary.BigEndian.Uint32(response[currPossition : currPossition+4]))
		currPossition += 4

		if size == 0 { // keepalive message
			log.Info("Received keepalice message")
			messages = append(messages, *NewPeerMessage(nil))
		} else {
			message := NewPeerMessage(response[currPossition : currPossition+size])
			log.WithFields(log.Fields{
				"code": message.code,
				"size": size,
			}).Debug("Peer message")
			currPossition = currPossition + size
			messages = append(messages, *message)
		}
	}

	return messages
}

func checkErr(err error, peer *Peer) {

	if err == nil {
		return
	} else if netError, ok := err.(net.Error); ok && netError.Timeout() {
		peer.logger.Warn("Peer conn timeout")
		return
	} else if io.EOF == err {
		peer.logger.Warn("Peer conn EOF")
		peer.PeerStatus.Valid = false
	}

	switch t := err.(type) {
	case *net.OpError:
		if t.Op == "dial" {
			peer.logger.Warn("Peer conn Unknown host")
			peer.PeerStatus.Valid = false
		} else if t.Op == "read" {
			peer.logger.Warn("Peer conn refused")
			peer.PeerStatus.Valid = false
		}

	case syscall.Errno:
		if t == syscall.ECONNREFUSED {
			peer.logger.Warn("Peer conn refuse 2")
			peer.PeerStatus.Valid = false
		}

	default:
		peer.logger.WithField("err", err).Warn("Unknown error")
	}
}

func readPeerConn(peer *Peer) ([]byte, error) {
	sizeDat := make([]byte, 4)
	peer.Conn.SetReadDeadline(time.Now().Add(peerTimeout))
	n, err := peer.Conn.Read(sizeDat)
	peer.logger.WithField("read", n).Debug("readPeerConn - reading message size from conn")

	if err != nil {
		checkErr(err, peer)
		return nil, err
	}

	if n != 4 {
		return nil, errors.New("Torrent message read error. Read data" + strconv.Itoa(n))
	}
	messageSize := binary.BigEndian.Uint32(sizeDat)
	if messageSize == 0 {
		return make([]byte, 0), nil
	}
	peer.logger.WithField("size", messageSize).Debug("Read peer message length")

	payload := make([]byte, messageSize)
	response := make([]byte, 0, messageSize+4)

	peer.Conn.SetReadDeadline(time.Now().Add(peerTimeout))
	n, err = io.ReadFull(peer.Conn, payload)
	peer.logger.WithField("read", n).Debug("readPeerConn - reading message payload from conn")

	if err != nil {
		checkErr(err, peer)
		return nil, err
	}

	response = append(response, sizeDat...)
	response = append(response, payload...)

	return response, nil
}

func (peer *Peer) connect() error {
	peer.logger.Info("Connecting to peer")
	if peer.Conn == nil {
		peer.start = time.Now()
		conn, err := net.DialTimeout("tcp", peer.Url, time.Second*5)
		peer.Conn = conn
		return err
	}

	return nil
}

func (peer *Peer) Announce() error {
	err := peer.connect()
	if err != nil {
		return err
	}
	peer.logger.Info("Sending announce message")

	handshake := peer.Torrent.CreateHandshake()
	peer.Conn.SetDeadline(time.Now().Add(time.Second * 5))
	peer.Conn.Write(handshake)

	peer.Conn.SetDeadline(time.Now().Add(time.Second * 5))
	response := readConn(peer.Conn)

	read := len(response)
	peer.logger.WithField("length", read).Info("Read handshake message")
	valid := checkHandshake(response, peer.Torrent.Hash, peer.Torrent.PeerId)

	if valid {
		messages := readResponse(response[68:])
		for _, message := range messages {
			peer.handlePeerMesssage(&message)
		}

		return nil
	} else {
		peer.logger.Warn("Peer handshake invalid")
		return errors.New("Peer handshake invalid")
	}
}
