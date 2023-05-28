package peer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"time"

	"github.com/anivanovic/gotit/pkg/gotitnet"
	"github.com/anivanovic/gotit/pkg/torrent"
	"github.com/anivanovic/gotit/pkg/util"

	"github.com/bits-and-blooms/bitset"
	"go.uber.org/zap"
)

var log = zap.L()

type PiecesSource interface {
	Next(bitset *bitset.BitSet) (uint, bool)
}

type Peer struct {
	Id           int
	AddrPort     netip.AddrPort
	conn         *gotitnet.TimeoutConn
	Bitset       *bitset.BitSet
	PeerStatus   *PeerStatus
	ClientStatus *PeerStatus
	lastMsgSent  time.Time
	logger       *zap.Logger
	piecesQueue  *torrent.PiecesQueue
	piecesSource PiecesSource

	blockIndx uint
	pieceIndx uint
	blockNum  uint

	writterCh chan<- *util.PeerMessage
}

type PeerStatus struct {
	Choking    bool
	Interested bool
	Valid      bool
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
	msg := util.CreatePieceMessage(uint32(peer.pieceIndx), uint32(beginOffset), uint32(torrent.BlockLength))

	peer.logger.Info("created piece request",
		zap.Uint("piece", peer.pieceIndx),
		zap.Uint("offset", beginOffset),
		zap.Uint("length", torrent.BlockLength),
	)

	peer.blockIndx++
	return msg
}

func newPeerStatus() *PeerStatus {
	return &PeerStatus{true, false, true}
}

func NewPeer(
	ip netip.AddrPort,
	torrent *torrent.Torrent,
	piecesQueue *torrent.PiecesQueue,
	writterCh chan<- *util.PeerMessage,
) *Peer {
	return &Peer{
		Id:           rand.Int(),
		AddrPort:     ip,
		conn:         nil,
		PeerStatus:   newPeerStatus(),
		ClientStatus: newPeerStatus(),
		lastMsgSent:  time.Now(),
		logger:       log.With(zap.String("ip", ip.String())),
		blockIndx:    uint(0),
		piecesQueue:  piecesQueue,
		writterCh:    writterCh,
		piecesSource: torrent,
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
		return fmt.Errorf("peer connect: %w", err)
	}

	peer.sendMessage(ctx, torrent.CreateHandshake())
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

		peer.checkKeepAlive(ctx)

		if peer.PeerStatus.Choking {
			peer.ClientStatus.Interested = true
			interestedM := util.CreateInterestedMessage()
			peer.logger.Debug("Sending interested message")

			_, err := peer.sendMessage(ctx, interestedM)
			if err != nil {
				return
			}
		}

		if !peer.PeerStatus.Choking && !sentPieceMsg {
			if requestMsg == nil {
				requestMsg = peer.nextRequestMessage()
			}

			if requestMsg == nil {
				select {
				case <-time.After(time.Second * 2):
					continue
				case <-ctx.Done():
					return
				}
			}

			if _, err := peer.sendMessage(ctx, requestMsg); err != nil {
				peer.logger.
					Warn("error sending piece. sleeping 5 seconds",
						zap.Error(err))
				peer.piecesQueue.RequestFailed(requestMsg)
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

		peer.handlePeerMessage(util.NewPeerMessage(response))
	}
}

func (peer *Peer) nextRequestMessage() []byte {
	if peer.blockIndx >= peer.blockNum {
		// when finished with piece download check if we have failed
		// piece requests
		if req := peer.piecesQueue.FailedPieceMessage(); req != nil {
			return req
		}

		indx, found := peer.piecesSource.Next(peer.Bitset)
		if !found {
			// we do not have any piece to request from the peer
			return nil
		}

		peer.blockIndx = 0
		peer.pieceIndx = indx
	}

	return peer.createPieceMessage()
}

func (peer *Peer) checkKeepAlive(ctx context.Context) {
	if time.Since(peer.lastMsgSent).Minutes() >= 1.9 {
		peer.logger.Debug("Sending keep alive message")
		peer.sendMessage(ctx, make([]byte, 4)) // send 0
		peer.updateLastMsgSent()
	}
}

func (peer *Peer) updateLastMsgSent() {
	peer.lastMsgSent = time.Now()
}

func (peer *Peer) handlePeerMessage(message *util.PeerMessage) {
	// if keepalive wait 2 minutes and try again
	if message.Type == util.KeepaliveMessageType {
		peer.logger.Debug("Peer sent keepalive")
		return
	}

	switch message.Type {
	case util.BitfieldMessageType:
		peer.logger.Info("Peer sent bitfield message")
		peer.Bitset = message.Bitfield()
	case util.HaveMessageType:
		peer.logger.Debug("Peer sent have message")
		peer.Bitset.Set(uint(message.Index()))
	case util.InterestedMessageType:
		peer.logger.Debug("Peer sent interested message")
		peer.PeerStatus.Interested = true
		// return choke or unchoke
	case util.NotInterestedMessageType:
		peer.logger.Debug("Peer sent notInterested message")
		peer.PeerStatus.Interested = false
		// return choke
	case util.ChokeMessageType:
		peer.logger.Debug("Peer sent choke message")
		peer.PeerStatus.Choking = true
		time.Sleep(time.Second * 30)
	case util.UnchokeMessageType:
		peer.logger.Debug("Peer sent unchoke message")
		peer.PeerStatus.Choking = false
	case util.RequestMessageType:
		peer.logger.Debug("Peer sent request message")
	case util.PieceMessageType:
		peer.logger.Debug("Peer sent piece message")
		//peer.mng.UpdateStatus(uint64(torrent.BlockLength), 0)
		peer.writterCh <- message
	case util.CancelMessageType:
		peer.logger.Debug("Peer sent cancel message")
	default:
		peer.logger.Sugar().Debugf("Peer sent wrong code %d", message.Type)
	}
}

func (peer *Peer) sendMessage(ctx context.Context, message []byte) (int, error) {
	n, err := peer.conn.Write(ctx, message)
	peer.logger.Debug("sendMessage to peer",
		zap.Int("written", n),
		zap.Error(err))
	return n, err
}
