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

	blockIdx uint
	pieceIdx uint
	blockNum uint

	writeCh chan<- *util.PeerMessage
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

	ressCode := handshake[0]
	protocolSignature := string(handshake[1:20])
	reservedBytes := binary.BigEndian.Uint64(handshake[20:28])
	sentHash := handshake[28:48]
	sentPeerId := handshake[48:68]

	return ressCode != 19 ||
		protocolSignature != string(torrent.BittorrentProto[:]) ||
		reservedBytes != 0 ||
		!bytes.Equal(sentHash, hash) ||
		!bytes.Equal(sentPeerId, peerId)
}

func (p *Peer) createPieceMessage() []byte {
	beginOffset := p.blockIdx * torrent.BlockLength
	msg := util.CreatePieceMessage(uint32(p.pieceIdx), uint32(beginOffset), uint32(torrent.BlockLength))

	p.logger.Info("created piece request",
		zap.Uint("piece", p.pieceIdx),
		zap.Uint("offset", beginOffset),
		zap.Uint("length", torrent.BlockLength),
	)

	p.blockIdx++
	return msg
}

func newPeerStatus() *PeerStatus {
	return &PeerStatus{true, false, true}
}

func NewPeer(
	ip netip.AddrPort,
	torrent *torrent.Torrent,
	piecesQueue *torrent.PiecesQueue,
	writeCh chan<- *util.PeerMessage,
	logger *zap.Logger,
) *Peer {
	return &Peer{
		Id:           rand.Int(),
		AddrPort:     ip,
		conn:         nil,
		PeerStatus:   newPeerStatus(),
		ClientStatus: newPeerStatus(),
		lastMsgSent:  time.Now(),
		logger:       logger.With(zap.String("ip", ip.String())),
		blockIdx:     uint(0),
		piecesQueue:  piecesQueue,
		writeCh:      writeCh,
		piecesSource: torrent,
	}
}

func (p *Peer) connect() error {
	var err error
	p.lastMsgSent = time.Now()
	p.conn, err = gotitnet.NewTimeoutConn("tcp", p.AddrPort.String(), gotitnet.PeerTimeout)
	return err
}

func (p *Peer) Announce(ctx context.Context, torrent *torrent.Torrent) error {
	err := p.connect()
	if err != nil {
		return fmt.Errorf("peer connect: %w", err)
	}

	p.sendMessage(ctx, torrent.CreateHandshake())
	response, err := p.conn.ReadPeerHandshake(ctx)
	if err != nil {
		return err
	}

	if valid := checkHandshake(response, torrent.Hash, torrent.PeerId); !valid {
		return errors.New("peer handshake invalid")
	}

	p.logger.Info("announce to peer successful")
	return nil
}

// Run communicates with remote peer and downloads torrent pieces.
func (p *Peer) Run(ctx context.Context) {
	var requestMsg []byte
	sentPieceMsg := false
	sleepTime := time.Second * 0
	for {
		if p.PeerStatus.Choking {
			sleepTime = 30
		}
		if sleepTime != 0 {
			sleep(ctx, sleepTime)
			sleepTime = time.Second * 0
			if ctx.Err() != nil {
				return
			}
		}

		p.checkKeepAlive(ctx)

		if p.PeerStatus.Choking {
			p.ClientStatus.Interested = true
			interestedM := util.CreateInterestedMessage()
			p.logger.Debug("Sending interested message")

			_, err := p.sendMessage(ctx, interestedM)
			if err != nil {
				return
			}
		}

		if !p.PeerStatus.Choking && !sentPieceMsg {
			if requestMsg == nil {
				requestMsg = p.nextRequestMessage()
			}

			if requestMsg == nil {
				sleepTime = time.Second * 2
				continue
			}

			if _, err := p.sendMessage(ctx, requestMsg); err != nil {
				p.logger.
					Warn("error sending piece. sleeping 5 seconds",
						zap.Error(err))
				p.piecesQueue.RequestFailed(requestMsg)
				sleepTime = time.Second * 5
				continue
			}
			sentPieceMsg = true
		}

		response, err := p.conn.ReadPeerMessage(ctx)
		if err != nil {
			if sentPieceMsg {
				p.piecesQueue.RequestFailed(requestMsg)
			}
			return
		}
		if sentPieceMsg {
			requestMsg = nil
			sentPieceMsg = false
		}

		p.handlePeerMessage(util.NewPeerMessage(response))
	}
}

func (p *Peer) nextRequestMessage() []byte {
	if p.blockIdx >= p.blockNum {
		// when finished with piece download check if we have failed
		// piece requests
		if req := p.piecesQueue.FailedPieceMessage(); req != nil {
			return req
		}

		indx, found := p.piecesSource.Next(p.Bitset)
		if !found {
			// we do not have any piece to request from the peer
			return nil
		}

		p.blockIdx = 0
		p.pieceIdx = indx
	}

	return p.createPieceMessage()
}

func (p *Peer) Close() error {
	return p.conn.Close()
}

func (p *Peer) checkKeepAlive(ctx context.Context) {
	if time.Since(p.lastMsgSent).Minutes() >= 1.9 {
		p.logger.Debug("Sending keep alive message")
		p.sendMessage(ctx, make([]byte, 4)) // send 0
		p.updateLastMsgSent()
	}
}

func (p *Peer) updateLastMsgSent() {
	p.lastMsgSent = time.Now()
}

func (p *Peer) handlePeerMessage(message *util.PeerMessage) {
	// if keepalive wait 2 minutes and try again
	if message.Type == util.KeepaliveMessageType {
		p.logger.Debug("Peer sent keepalive")
		return
	}

	switch message.Type {
	case util.BitfieldMessageType:
		p.logger.Info("Peer sent bitfield message")
		p.Bitset = message.Bitfield()
	case util.HaveMessageType:
		p.logger.Debug("Peer sent have message")
		p.Bitset.Set(uint(message.Index()))
	case util.InterestedMessageType:
		p.logger.Debug("Peer sent interested message")
		p.PeerStatus.Interested = true
		// return choke or unchoke
	case util.NotInterestedMessageType:
		p.logger.Debug("Peer sent notInterested message")
		p.PeerStatus.Interested = false
		// return choke
	case util.ChokeMessageType:
		p.logger.Debug("Peer sent choke message")
		p.PeerStatus.Choking = true
	case util.UnchokeMessageType:
		p.logger.Debug("Peer sent unchoke message")
		p.PeerStatus.Choking = false
	case util.RequestMessageType:
		p.logger.Debug("Peer sent request message")
	case util.PieceMessageType:
		p.logger.Debug("Peer sent piece message")
		//peer.mng.UpdateStatus(uint64(torrent.BlockLength), 0)
		p.writeCh <- message
	case util.CancelMessageType:
		p.logger.Debug("Peer sent cancel message")
	default:
		p.logger.Sugar().Debugf("Peer sent wrong code %d", message.Type)
	}
}

func (p *Peer) sendMessage(ctx context.Context, message []byte) (int, error) {
	n, err := p.conn.Write(ctx, message)
	p.logger.Debug("sendMessage to peer",
		zap.Int("written", n),
		zap.Error(err))
	return n, err
}

func sleep(ctx context.Context, duration time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}
}
