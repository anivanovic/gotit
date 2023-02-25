package peer

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/netip"
	"time"

	"github.com/anivanovic/gotit/pkg/gotitnet"
	"github.com/anivanovic/gotit/pkg/torrent"
	"github.com/anivanovic/gotit/pkg/util"

	"errors"

	"math/rand"

	"github.com/bits-and-blooms/bitset"
	"go.uber.org/zap"
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
	keepalive     = 99
)

var log = zap.L()

type Peer struct {
	Id           int
	AddrPort     netip.AddrPort
	conn         *gotitnet.TimeoutConn
	Bitset       *bitset.BitSet
	PeerStatus   *PeerStatus
	ClientStatus *PeerStatus
	torrent      *torrent.Torrent
	lastMsgSent  time.Time
	logger       *zap.Logger
	piecesQueue  *torrent.PiecesQueue

	blockIndx uint
	pieceIndx uint

	writterCh chan<- *util.PeerMessage
	doneCh    chan<- struct{}
}

type PeerStatus struct {
	Choking    bool
	Interested bool
	Valid      bool
}

var keepalivePeerMessage = &util.PeerMessage{
	Size:    0,
	Code:    keepalive,
	Payload: nil,
}

func NewPeerMessage(data []byte) *util.PeerMessage {
	if len(data) == 0 { // keepalive message
		return keepalivePeerMessage
	}

	return &util.PeerMessage{
		Size:    uint32(len(data)),
		Code:    data[0],
		Payload: data[1:],
	}
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
	binary.Write(message, binary.BigEndian, peer.Bitset.Len())
	binary.Write(message, binary.BigEndian, uint8(bitfield))
	binary.Write(message, binary.BigEndian, peer.Bitset.Bytes())

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
	log.Debug("Peer handshake message",
		zap.Uint8("resCode", ressCode),
		zap.String("protocol signature", protocolSignature),
		zap.Binary("hash", sentHash),
		zap.Binary("peerId", sentPeerId))

	return ressCode != 19 ||
		protocolSignature != string(torrent.BittorrentProto[:]) ||
		reservedBytes != 0 ||
		!bytes.Equal(sentHash, hash) ||
		!bytes.Equal(sentPeerId, peerId)
}

func (peer *Peer) createPieceMessage() []byte {
	beginOffset := peer.blockIndx * torrent.BlockLength
	message := &bytes.Buffer{}
	binary.Write(message, binary.BigEndian, uint32(13))
	binary.Write(message, binary.BigEndian, uint8(request))
	binary.Write(message, binary.BigEndian, uint32(peer.pieceIndx))
	binary.Write(message, binary.BigEndian, uint32(beginOffset))
	binary.Write(message, binary.BigEndian, uint32(torrent.BlockLength))

	peer.logger.Info("created piece request",
		zap.Uint("piece", peer.pieceIndx),
		zap.Uint("offset", beginOffset),
		zap.Uint("length", torrent.BlockLength),
	)

	peer.blockIndx++

	return message.Bytes()
}

func newPeerStatus() *PeerStatus {
	return &PeerStatus{true, false, true}
}

func NewPeer(
	ip netip.AddrPort,
	torrent *torrent.Torrent,
	piecesQueue *torrent.PiecesQueue,
	writterCh chan<- *util.PeerMessage,
	doneCh chan<- struct{},
) *Peer {
	return &Peer{
		Id:           rand.Int(),
		AddrPort:     ip,
		conn:         nil,
		PeerStatus:   newPeerStatus(),
		ClientStatus: newPeerStatus(),
		torrent:      torrent,
		lastMsgSent:  time.Now(),
		logger:       log.With(zap.String("ip", ip.String())),
		blockIndx:    uint(0),
		piecesQueue:  piecesQueue,
		writterCh:    writterCh,
		doneCh:       doneCh,
	}
}

func (peer *Peer) connect() error {
	var err error
	peer.lastMsgSent = time.Now()
	peer.conn, err = gotitnet.NewTimeoutConn("tcp", peer.AddrPort.String(), gotitnet.PeerTimeout)
	return err
}

func (peer *Peer) Announce(ctx context.Context, torrent *torrent.Torrent) error {
	err := peer.connect()
	if err != nil {
		return err
	}

	peer.sendMessage(torrent.CreateHandshake())
	response, err := peer.conn.ReadPeerHandshake(ctx)
	if err != nil {
		return err
	}

	if valid := checkHandshake(response, torrent.Hash, torrent.PeerId); !valid {
		return errors.New("peer handshake invalid")
	}

	peer.logger.Info("announce to peer successfull")
	return nil
}

// Run communicates with remote peer and downloads torrent pieces.
func (peer *Peer) Run(ctx context.Context) {
	var requestMsg []byte
	sentPieceMsg := false
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

		if !peer.PeerStatus.Choking && !sentPieceMsg {
			if requestMsg == nil {
				requestMsg = peer.nextRequestMessage(peer.torrent)
			}

			if requestMsg == nil && peer.torrent.Done() {
				peer.doneCh <- struct{}{}
				return
			}

			if requestMsg == nil {
				select {
				case <-time.After(time.Second * 10):
					continue
				case <-ctx.Done():
					return
				}
			}

			if _, err := peer.sendMessage(requestMsg); err != nil {
				peer.logger.
					Warn("error sending piece. sleeping 5 seconds",
						zap.Error(err))
				time.Sleep(time.Second * 5)
				continue
			}
			sentPieceMsg = true
		}

		response, err := peer.conn.ReadPeerMessage(ctx)
		if err != nil {
			if sentPieceMsg {
				peer.piecesQueue.RequestFailed(requestMsg)
			}
			return
		}
		if sentPieceMsg {
			requestMsg = nil
			sentPieceMsg = false
		}

		peer.handlePeerMessage(NewPeerMessage(response))
	}
}

func (peer *Peer) nextRequestMessage(torrent *torrent.Torrent) []byte {
	if peer.blockIndx >= uint(torrent.BlockNum()) {
		// when finished with piece download check if we have failed
		// piece requests
		if req := peer.piecesQueue.FailedPieceMessage(); req != nil {
			return req
		}

		indx, found := torrent.CreateNextRequestMessage(peer.Bitset)
		if !found {
			// we do not have any piece to request from the peer
			return nil
		}

		peer.blockIndx = 0
		peer.pieceIndx = indx
	}

	return peer.createPieceMessage()
}

func (peer *Peer) checkKeepAlive() {
	if time.Since(peer.lastMsgSent).Minutes() >= 1.9 {
		peer.logger.Debug("Sending keep alive message")
		peer.sendMessage(make([]byte, 4)) // send 0
		peer.updateLastMsgSent()
	}
}

func (peer *Peer) updateLastMsgSent() {
	peer.lastMsgSent = time.Now()
}

func (peer *Peer) handlePeerMessage(message *util.PeerMessage) {
	// if keepalive wait 2 minutes and try again
	if message.Size == 0 {
		peer.logger.Debug("Peer sent keepalive")
		return
	}

	switch message.Code {
	case bitfield:
		peer.logger.Info("Peer sent bitfield message")
		peer.Bitset = createBitset(message.Payload)
	case have:
		peer.logger.Debug("Peer sent have message")
		indx := uint(binary.BigEndian.Uint32(message.Payload))
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
		//peer.mng.UpdateStatus(uint64(torrent.BlockLength), 0)
		peer.writterCh <- message
	case cancel:
		peer.logger.Debug("Peer sent cancle message")
	default:
		peer.logger.Sugar().Debugf("Peer sent wrong code %d", message.Code)
	}
}

func createBitset(payload []byte) *bitset.BitSet {
	set := make([]uint64, 0)
	i := 0
	lenPayload := len(payload)
	for i+8 < lenPayload {
		data := binary.BigEndian.Uint64(payload[i : i+8])
		set = append(set, data)
		i += 8
	}
	if i < lenPayload {
		n := lenPayload - i
		missing := 8 - n
		data := payload[i:lenPayload]
		for i := 0; i < missing; i++ {
			data = append(data, 0)
		}
		last := binary.BigEndian.Uint64(data)
		set = append(set, last)
	}

	return bitset.From(set)
}

func (peer *Peer) sendMessage(message []byte) (int, error) {
	n, err := peer.conn.Write(context.TODO(), message)
	peer.logger.Debug("sendMessage to peer",
		zap.Int("written", n),
		zap.Error(err))
	return n, err
}
