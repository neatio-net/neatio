package p2p

import (
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/network/p2p/discover"
	"github.com/nio-net/nio/utilities/common/mclock"
	"github.com/nio-net/nio/utilities/event"
	"github.com/nio-net/nio/utilities/rlp"
)

const (
	baseProtocolVersion    = 5
	baseProtocolLength     = uint64(16)
	baseProtocolMaxMsgSize = 2 * 1024

	snappyProtocolVersion = 5

	pingInterval = 15 * time.Second
)

const (
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03

	BroadcastNewSideChainMsg = 0x04
	ConfirmNewSideChainMsg   = 0x05

	RefreshValidatorNodeInfoMsg = 0x06
	RemoveValidatorNodeInfoMsg  = 0x07
)

type protoHandshake struct {
	Version    uint64
	Name       string
	Caps       []Cap
	ListenPort uint64
	ID         discover.NodeID

	Rest []rlp.RawValue `rlp:"tail"`
}

type PeerEventType string

const (
	PeerEventTypeAdd PeerEventType = "add"

	PeerEventTypeDrop PeerEventType = "drop"

	PeerEventTypeMsgSend PeerEventType = "msgsend"

	PeerEventTypeMsgRecv PeerEventType = "msgrecv"

	PeerEventTypeRefreshValidator PeerEventType = "refreshvalidator"

	PeerEventTypeRemoveValidator PeerEventType = "removevalidator"
)

type PeerEvent struct {
	Type     PeerEventType   `json:"type"`
	Peer     discover.NodeID `json:"peer"`
	Error    string          `json:"error,omitempty"`
	Protocol string          `json:"protocol,omitempty"`
	MsgCode  *uint64         `json:"msg_code,omitempty"`
	MsgSize  *uint32         `json:"msg_size,omitempty"`
}

type Peer struct {
	rw      *conn
	running map[string]*protoRW
	log     log.Logger
	created mclock.AbsTime

	wg       sync.WaitGroup
	protoErr chan error
	closed   chan struct{}
	disc     chan DiscReason

	events *event.Feed

	srvProtocols *[]Protocol
}

func NewPeer(id discover.NodeID, name string, caps []Cap) *Peer {
	pipe, _ := net.Pipe()
	conn := &conn{fd: pipe, transport: nil, id: id, caps: caps, name: name}
	peer := newPeer(conn, nil)
	close(peer.closed)
	return peer
}

func (p *Peer) ID() discover.NodeID {
	return p.rw.id
}

func (p *Peer) Name() string {
	return p.rw.name
}

func (p *Peer) Caps() []Cap {

	return p.rw.caps
}

func (p *Peer) RemoteAddr() net.Addr {
	return p.rw.fd.RemoteAddr()
}

func (p *Peer) LocalAddr() net.Addr {
	return p.rw.fd.LocalAddr()
}

func (p *Peer) Disconnect(reason DiscReason) {
	select {
	case p.disc <- reason:
	case <-p.closed:
	}
}

func (p *Peer) String() string {
	return fmt.Sprintf("Peer %x %v", p.rw.id[:8], p.RemoteAddr())
}

func (p *Peer) Inbound() bool {
	return p.rw.flags&inboundConn != 0
}

func newPeer(conn *conn, protocols []Protocol) *Peer {
	protomap := matchProtocols(protocols, conn.caps, conn)
	p := &Peer{
		rw:       conn,
		running:  protomap,
		created:  mclock.Now(),
		disc:     make(chan DiscReason),
		protoErr: make(chan error, len(protomap)+1),
		closed:   make(chan struct{}),
		log:      log.New("id", conn.id, "conn", conn.flags),
	}
	return p
}

func (p *Peer) Log() log.Logger {
	return p.log
}

func (p *Peer) run() (remoteRequested bool, err error) {
	var (
		writeStart = make(chan struct{}, 1)
		writeErr   = make(chan error, 1)
		readErr    = make(chan error, 1)
		reason     DiscReason
	)
	p.wg.Add(2)
	go p.readLoop(readErr)
	go p.pingLoop()

	writeStart <- struct{}{}
	p.startProtocols(writeStart, writeErr)

loop:
	for {
		select {
		case err = <-writeErr:

			if err != nil {
				reason = DiscNetworkError
				break loop
			}
			writeStart <- struct{}{}
		case err = <-readErr:
			if r, ok := err.(DiscReason); ok {
				remoteRequested = true
				reason = r
			} else {
				reason = DiscNetworkError
			}
			break loop
		case err = <-p.protoErr:
			reason = discReasonForError(err)
			break loop
		case err = <-p.disc:
			break loop
		}
	}

	close(p.closed)
	p.rw.close(reason)
	p.wg.Wait()
	return remoteRequested, err
}

func (p *Peer) pingLoop() {
	ping := time.NewTimer(pingInterval)
	defer p.wg.Done()
	defer ping.Stop()
	for {
		select {
		case <-ping.C:
			if err := SendItems(p.rw, pingMsg); err != nil {
				p.protoErr <- err
				return
			}
			ping.Reset(pingInterval)
		case <-p.closed:
			return
		}
	}
}

func (p *Peer) readLoop(errc chan<- error) {
	defer p.wg.Done()
	for {
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		msg.ReceivedAt = time.Now()
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}

func (p *Peer) handle(msg Msg) error {
	switch {
	case msg.Code == pingMsg:
		msg.Discard()
		go SendItems(p.rw, pongMsg)
	case msg.Code == discMsg:
		var reason [1]DiscReason

		rlp.Decode(msg.Payload, &reason)
		return reason[0]
	case msg.Code == BroadcastNewSideChainMsg:

		var chainId string
		if err := msg.Decode(&chainId); err != nil {
			return err
		}

		p.log.Infof("Got new side chain msg from Peer %v, Before add protocol. Caps %v, Running Proto %+v", p.String(), p.Caps(), p.Info().Protocols)

		newRunning := p.checkAndUpdateProtocol(chainId)
		if newRunning {

			go Send(p.rw, ConfirmNewSideChainMsg, chainId)
		}

		p.log.Infof("Got new side chain msg After add protocol. Caps %v, Running Proto %+v", p.Caps(), p.Info().Protocols)

	case msg.Code == ConfirmNewSideChainMsg:

		var chainId string
		if err := msg.Decode(&chainId); err != nil {
			return err
		}
		p.log.Infof("Got confirm msg from Peer %v, Before add protocol. Caps %v, Running Proto %+v", p.String(), p.Caps(), p.Info().Protocols)
		p.checkAndUpdateProtocol(chainId)
		p.log.Infof("Got confirm msg After add protocol. Caps %v, Running Proto %+v", p.Caps(), p.Info().Protocols)

	case msg.Code == RefreshValidatorNodeInfoMsg:
		p.log.Debug("Got refresh validation node infomation")
		var valNodeInfo P2PValidatorNodeInfo
		if err := msg.Decode(&valNodeInfo); err != nil {
			p.log.Debugf("decode error: %v", err)
			return err
		}
		p.log.Debugf("validation node address: %x", valNodeInfo.Validator.Address)

		if valNodeInfo.Original && p.Info().ID == valNodeInfo.Node.ID.String() {
			valNodeInfo.Node.IP = p.RemoteAddr().(*net.TCPAddr).IP
		}
		valNodeInfo.Original = false

		p.log.Debugf("validator node info: %v", valNodeInfo)

		data, err := rlp.EncodeToBytes(valNodeInfo)
		if err != nil {
			p.log.Debugf("encode error: %v", err)
			return err
		}
		p.events.Send(&PeerEvent{
			Type:     PeerEventTypeRefreshValidator,
			Peer:     p.ID(),
			Protocol: string(data),
		})

		p.log.Debugf("RefreshValidatorNodeInfoMsg handled")

	case msg.Code == RemoveValidatorNodeInfoMsg:
		p.log.Debug("Got remove validation node infomation")
		var valNodeInfo P2PValidatorNodeInfo
		if err := msg.Decode(&valNodeInfo); err != nil {
			p.log.Debugf("decode error: %v", err)
			return err
		}
		p.log.Debugf("validation node address: %x", valNodeInfo.Validator.Address)

		if valNodeInfo.Original {
			valNodeInfo.Node.IP = p.RemoteAddr().(*net.TCPAddr).IP
			valNodeInfo.Original = false
		}
		p.log.Debugf("validator node info: %v", valNodeInfo)

		data, err := rlp.EncodeToBytes(valNodeInfo)
		if err != nil {
			p.log.Debugf("encode error: %v", err)
			return err
		}
		p.events.Send(&PeerEvent{
			Type:     PeerEventTypeRemoveValidator,
			Peer:     p.ID(),
			Protocol: string(data),
		})

		p.log.Debug("RemoveValidatorNodeInfoMsg handled")

	case msg.Code < baseProtocolLength:

		return msg.Discard()
	default:

		proto, err := p.getProto(msg.Code)
		if err != nil {
			return fmt.Errorf("msg code out of range: %v", msg.Code)
		}
		select {
		case proto.in <- msg:
			return nil
		case <-p.closed:
			return io.EOF
		}
	}
	return nil
}

func (p *Peer) checkAndUpdateProtocol(chainId string) bool {

	sideProtocolName := "neatchain_" + chainId

	if _, exist := p.running[sideProtocolName]; exist {
		p.log.Infof("Side Chain %v is already running on peer", sideProtocolName)
		return false
	}

	sideProtocolOffset := getLargestOffset(p.running)
	if match, protoRW := matchServerProtocol(*p.srvProtocols, sideProtocolName, sideProtocolOffset, p.rw); match {

		p.startSideChainProtocol(protoRW)

		p.running[sideProtocolName] = protoRW

		protoCap := protoRW.cap()
		capExist := false
		for _, cap := range p.rw.caps {
			if cap.Name == protoCap.Name && cap.Version == protoCap.Version {
				capExist = true
			}
		}
		if !capExist {
			p.rw.caps = append(p.rw.caps, protoCap)
		}
		return true
	}

	p.log.Infof("No Local Server Protocol matched, perhaps local server has not start the side chain %v yet.", sideProtocolName)
	return false
}

func countMatchingProtocols(protocols []Protocol, caps []Cap) int {
	n := 0
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				n++
			}
		}
	}
	return n
}

func matchProtocols(protocols []Protocol, caps []Cap, rw MsgReadWriter) map[string]*protoRW {
	sort.Sort(capsByNameAndVersion(caps))
	offset := baseProtocolLength
	result := make(map[string]*protoRW)

outer:
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {

				if old := result[cap.Name]; old != nil {
					offset -= old.Length
				}

				result[cap.Name] = &protoRW{Protocol: proto, offset: offset, in: make(chan Msg), w: rw}
				offset += proto.Length

				continue outer
			}
		}
	}
	return result
}

func matchServerProtocol(protocols []Protocol, name string, offset uint64, rw MsgReadWriter) (bool, *protoRW) {
	for _, proto := range protocols {
		if proto.Name == name {

			return true, &protoRW{Protocol: proto, offset: offset, in: make(chan Msg), w: rw}
		}
	}
	return false, nil
}

func getLargestOffset(running map[string]*protoRW) uint64 {
	var largestOffset uint64 = 0
	for _, proto := range running {
		offsetEnd := proto.offset + proto.Length
		if offsetEnd > largestOffset {
			largestOffset = offsetEnd
		}
	}
	return largestOffset
}

func (p *Peer) startProtocols(writeStart <-chan struct{}, writeErr chan<- error) {
	p.wg.Add(len(p.running))
	for _, proto := range p.running {
		proto := proto
		proto.closed = p.closed
		proto.wstart = writeStart
		proto.werr = writeErr
		var rw MsgReadWriter = proto
		if p.events != nil {
			rw = newMsgEventer(rw, p.events, p.ID(), proto.Name)
		}
		p.log.Trace(fmt.Sprintf("Starting protocol %s/%d", proto.Name, proto.Version))
		go func() {
			err := proto.Run(p, rw)
			if err == nil {
				p.log.Trace(fmt.Sprintf("Protocol %s/%d returned", proto.Name, proto.Version))
				err = errProtocolReturned
			} else if err != io.EOF {
				p.log.Trace(fmt.Sprintf("Protocol %s/%d failed", proto.Name, proto.Version), "err", err)
			}
			p.protoErr <- err
			p.wg.Done()
		}()
	}
}

func (p *Peer) startSideChainProtocol(proto *protoRW) {
	p.wg.Add(1)

	proto.closed = p.closed
	proto.wstart = p.running["neatio"].wstart
	proto.werr = p.running["neatio"].werr

	var rw MsgReadWriter = proto
	if p.events != nil {
		rw = newMsgEventer(rw, p.events, p.ID(), proto.Name)
	}
	p.log.Trace(fmt.Sprintf("Starting protocol %s/%d", proto.Name, proto.Version))
	go func() {
		err := proto.Run(p, rw)
		if err == nil {
			p.log.Trace(fmt.Sprintf("Protocol %s/%d returned", proto.Name, proto.Version))
			err = errProtocolReturned
		} else if err != io.EOF {
			p.log.Trace(fmt.Sprintf("Protocol %s/%d failed", proto.Name, proto.Version), "err", err)
		}
		p.protoErr <- err
		p.wg.Done()
	}()
}

func (p *Peer) getProto(code uint64) (*protoRW, error) {
	for _, proto := range p.running {
		if code >= proto.offset && code < proto.offset+proto.Length {
			return proto, nil
		}
	}
	return nil, newPeerError(errInvalidMsgCode, "%d", code)
}

type protoRW struct {
	Protocol
	in     chan Msg
	closed <-chan struct{}
	wstart <-chan struct{}
	werr   chan<- error
	offset uint64
	w      MsgWriter
}

func (rw *protoRW) WriteMsg(msg Msg) (err error) {
	if msg.Code >= rw.Length {
		return newPeerError(errInvalidMsgCode, "not handled")
	}
	msg.Code += rw.offset
	select {
	case <-rw.wstart:
		err = rw.w.WriteMsg(msg)

		rw.werr <- err
	case <-rw.closed:
		err = fmt.Errorf("shutting down")
	}
	return err
}

func (rw *protoRW) ReadMsg() (Msg, error) {
	select {
	case msg := <-rw.in:
		msg.Code -= rw.offset
		return msg, nil
	case <-rw.closed:
		return Msg{}, io.EOF
	}
}

type PeerInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Caps    []string `json:"caps"`
	Network struct {
		LocalAddress  string `json:"localAddress"`
		RemoteAddress string `json:"remoteAddress"`
		Inbound       bool   `json:"inbound"`
		Trusted       bool   `json:"trusted"`
		Static        bool   `json:"static"`
	} `json:"network"`
	Protocols map[string]interface{} `json:"protocols"`
}

func (p *Peer) Info() *PeerInfo {

	var caps []string
	for _, cap := range p.Caps() {
		caps = append(caps, cap.String())
	}

	info := &PeerInfo{
		ID:        p.ID().String(),
		Name:      p.Name(),
		Caps:      caps,
		Protocols: make(map[string]interface{}),
	}
	info.Network.LocalAddress = p.LocalAddr().String()
	info.Network.RemoteAddress = p.RemoteAddr().String()
	info.Network.Inbound = p.rw.is(inboundConn)
	info.Network.Trusted = p.rw.is(trustedConn)
	info.Network.Static = p.rw.is(staticDialedConn)

	for _, proto := range p.running {
		protoInfo := interface{}("unknown")
		if query := proto.Protocol.PeerInfo; query != nil {
			if metadata := query(p.ID()); metadata != nil {
				protoInfo = metadata
			} else {
				protoInfo = "handshake"
			}
		}
		info.Protocols[proto.Name] = protoInfo
	}
	return info
}
