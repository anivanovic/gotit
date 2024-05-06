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

var (
	bittorrentProto = [19]byte{'B', 'i', 't', 'T', 'o', 'r', 'r', 'e', 'n', 't', ' ', 'p', 'r', 'o', 't', 'o', 'c', 'o', 'l'}
	clientIdPrefix  = [8]byte{'-', 'G', 'O', '0', '1', '0', '0', '-'}
)

type PiecesSource interface {
	Next(bitset *bitset.BitSet) (uint, bool)
}

type PieceChecker interface {
	CheckPiece([]byte, int) bool
}

type Peer struct {
	Id           int
	AddrPort     netip.AddrPort
	conn         *gotitnet.TimeoutConn
	Bitset       *bitset.BitSet
	PeerStatus   *Status
	ClientStatus *Status
	lastMsgSent  time.Time
	piecesQueue  *torrent.PiecesQueue
	piecesSource PiecesSource
	pieceChecker PieceChecker

	torrent *torrent.Torrent

	logger *zap.Logger

	blockIdx uint
	pieceIdx uint
	blockNum uint

	writeCh chan<- *util.PeerMessage
}

type Status struct {
	Choked     bool
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
		protocolSignature != string(bittorrentProto[:]) ||
		reservedBytes != 0 ||
		!bytes.Equal(sentHash, hash) ||
		!bytes.Equal(sentPeerId, peerId)
}

func (p *Peer) createPieceMessage() *util.PeerMessage {
	beginOffset := p.blockIdx * torrent.BlockLength
	msg := util.CreatePieceMessage(uint32(p.pieceIdx), uint32(beginOffset), uint32(torrent.BlockLength))

	p.logger.Debug("created piece request",
		zap.Uint("piece", p.pieceIdx),
		zap.Uint("offset", beginOffset),
		zap.Uint("length", torrent.BlockLength),
	)

	p.blockIdx++
	return msg
}

func newPeerStatus() *Status {
	return &Status{true, false, true}
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
		Bitset:       torrent.EmptyBitset(),
	}
}

func (p *Peer) connect() error {
	var err error
	p.conn, err = gotitnet.NewTimeoutConn("tcp", p.AddrPort.String(), gotitnet.PeerTimeout)
	if err != nil {
		return err
	}

	p.lastMsgSent = time.Now()
	return nil
}

func (p *Peer) Announce(torrent *torrent.Torrent) error {
	err := p.connect()
	if err != nil {
		return fmt.Errorf("peer connect: %w", err)
	}

	if _, err := p.send(createHandshake(torrent.Hash)); err != nil {
		return fmt.Errorf("peer handshake: %w", err)
	}

	response, err := p.conn.ReadPeerHandshake()
	if err != nil {
		return err
	}

	if valid := checkHandshake(response, torrent.Hash, ClientId); !valid {
		return errors.New("peer handshake invalid")
	}

	p.logger.Info("announce to peer successful")
	return nil
}

// Run communicates with remote peer and downloads torrent pieces.
func (p *Peer) Run(ctx context.Context) {
	var requestMsg *util.PeerMessage
	sentPieceMsg := false

	p.SendInterested()
	p.SendUnchoke()

	for {
		if !p.ClientStatus.Choked && !sentPieceMsg {
			requestMsg = p.nextRequestMessage()
			if requestMsg == nil {
				time.Sleep(time.Second * 2)
				continue
			}

			if _, err := p.sendMessage(requestMsg); err != nil {
				p.logger.
					Debug("error sending piece. sleeping 5 seconds",
						zap.Error(err))
				p.piecesQueue.RequestFailed(requestMsg)
				continue
			}

			sentPieceMsg = true
		}

		response, err := p.conn.ReadPeerMessage()
		if err != nil {
			if sentPieceMsg {
				p.piecesQueue.RequestFailed(requestMsg)
				sentPieceMsg = false
			}

			continue
		}

		p.handlePeerMessage(util.NewPeerMessage(response))
	}
}

func (p *Peer) nextRequestMessage() *util.PeerMessage {
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

func (p *Peer) checkKeepAlive() {
	if time.Since(p.lastMsgSent).Minutes() >= 1.9 {
		p.logger.Debug("Sending keep alive message")
		p.sendMessage(util.KeepalivePeerMessage)
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
		p.logger.Debug("Peer sent bitfield message", zap.Int("peerId", p.Id))
		p.Bitset = message.Bitfield()
	case util.HaveMessageType:
		p.logger.Debug("Peer sent have message", zap.Int("peerId", p.Id))
		p.Bitset.Set(uint(message.Index()))
	case util.InterestedMessageType:
		p.logger.Debug("Peer sent interested message", zap.Int("peerId", p.Id))
		p.PeerStatus.Interested = true
		// return choke or unchoke
	case util.NotInterestedMessageType:
		p.logger.Debug("Peer sent notInterested message", zap.Int("peerId", p.Id))
		p.PeerStatus.Interested = false
		// return choke
	case util.ChokeMessageType:
		p.logger.Debug("Peer sent choke message", zap.Int("peerId", p.Id))
		p.ClientStatus.Choked = true
	case util.UnchokeMessageType:
		p.logger.Debug("Peer sent unchoke message", zap.Int("peerId", p.Id))
		p.ClientStatus.Choked = false
	case util.RequestMessageType:
		p.logger.Debug("Peer sent request message", zap.Int("peerId", p.Id))
	case util.PieceMessageType:
		p.logger.Debug("Peer sent piece message", zap.Int("peerId", p.Id))
		p.handlePieceMessage(message)
	case util.CancelMessageType:
		p.logger.Debug("Peer sent cancel message")
	default:
		p.logger.Error("peer sent unrecognized message",
			zap.Int("peerId", p.Id),
			zap.Int("msgCode", int(message.Type)))
	}
}

func (p *Peer) sendMessage(msg *util.PeerMessage) (int, error) {
	return p.conn.WriteMsg(msg)
}

func (p *Peer) send(data []byte) (int, error) {
	return p.conn.Write(data)
}

func (p *Peer) SendUnchoke() {
	if !p.PeerStatus.Interested {
		return
	}

	if _, err := p.sendMessage(util.CreateUnchokeMessage()); err != nil {
		return
	}
	p.PeerStatus.Choked = false
}

func (p *Peer) SendInterested() {
	p.sendMessage(util.CreateInterestedMessage())
}

func (p *Peer) handlePieceMessage(message *util.PeerMessage) {
	if !p.pieceChecker.CheckPiece(message.Data(), int(message.Index())) {
		p.logger.Error("Discarding corrupted piece. Sha1 check failed.", zap.Uint32("index", message.Index()))
		return
	}

	p.writeCh <- message
	p.Bitset.Set(uint(message.Index()))
}

func sleep(ctx context.Context, duration time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}
}

func createHandshake(hash []byte) []byte {
	buf := new(bytes.Buffer)

	// 19 - as number of letters in protocol type string
	_ = binary.Write(buf, binary.BigEndian, uint8(len(bittorrentProto)))
	_ = binary.Write(buf, binary.BigEndian, bittorrentProto)
	_ = binary.Write(buf, binary.BigEndian, uint64(0))
	_ = binary.Write(buf, binary.BigEndian, hash)
	_ = binary.Write(buf, binary.BigEndian, ClientId)

	return buf.Bytes()
}

// implemented BEP20
var ClientId = createClientId()

func createClientId() []byte {
	peerId := make([]byte, 20)
	copy(peerId, clientIdPrefix[:])
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Read(peerId[len(clientIdPrefix):])
	return peerId
}
