package p2p

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/network/p2p/discover"
	"github.com/neatio-net/neatio/network/p2p/discv5"
	"github.com/neatio-net/neatio/network/p2p/nat"
	"github.com/neatio-net/neatio/network/p2p/netutil"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/common/mclock"
	"github.com/neatio-net/neatio/utilities/event"
	"github.com/neatio-net/neatio/utilities/rlp"
)

const (
	defaultDialTimeout = 15 * time.Second

	maxActiveDialTasks     = 16
	defaultMaxPendingPeers = 50
	defaultDialRatio       = 3

	frameReadTimeout = 30 * time.Second

	frameWriteTimeout = 20 * time.Second
)

var errServerStopped = errors.New("server stopped")

type Config struct {
	PrivateKey *ecdsa.PrivateKey `toml:"-"`

	MaxPeers int

	MaxPendingPeers int `toml:",omitempty"`

	DialRatio int `toml:",omitempty"`

	NoDiscovery bool

	DiscoveryV5 bool `toml:",omitempty"`

	Name string `toml:"-"`

	BootstrapNodes []*discover.Node

	BootstrapNodesV5 []*discv5.Node `toml:",omitempty"`

	StaticNodes []*discover.Node

	TrustedNodes []*discover.Node

	LocalValidators []P2PValidator

	Validators map[P2PValidator]*P2PValidatorNodeInfo

	NetRestrict *netutil.Netlist `toml:",omitempty"`

	NodeDatabase string `toml:",omitempty"`

	Protocols []Protocol `toml:"-"`

	ListenAddr string

	NAT nat.Interface `toml:",omitempty"`

	Dialer NodeDialer `toml:"-"`

	NoDial bool `toml:",omitempty"`

	EnableMsgEvents bool

	Logger log.Logger `toml:",omitempty"`
}

type NodeInfoToSend struct {
	valNodeInfo *P2PValidatorNodeInfo
	action      uint64
	p           *Peer
}

type Server struct {
	Config

	newTransport func(net.Conn) transport
	newPeerHook  func(*Peer)

	lock    sync.Mutex
	running bool

	ntab         discoverTable
	listener     net.Listener
	ourHandshake *protoHandshake
	lastLookup   time.Time
	DiscV5       *discv5.Network

	peerOp     chan peerOpFunc
	peerOpDone chan struct{}

	quit          chan struct{}
	addstatic     chan *discover.Node
	removestatic  chan *discover.Node
	posthandshake chan *conn
	addpeer       chan *conn
	delpeer       chan peerDrop
	loopWG        sync.WaitGroup
	peerFeed      event.Feed
	log           log.Logger

	events    chan *PeerEvent
	eventsSub event.Subscription

	nodeInfoLock sync.Mutex
	nodeInfoList []*NodeInfoToSend
}

type peerOpFunc func(map[discover.NodeID]*Peer)

type peerDrop struct {
	*Peer
	err       error
	requested bool
}

type connFlag int

const (
	dynDialedConn connFlag = 1 << iota
	staticDialedConn
	inboundConn
	trustedConn
)

type conn struct {
	fd net.Conn
	transport
	flags connFlag
	cont  chan error
	id    discover.NodeID
	caps  []Cap
	name  string
}

type transport interface {
	doEncHandshake(prv *ecdsa.PrivateKey, dialDest *discover.Node) (discover.NodeID, error)
	doProtoHandshake(our *protoHandshake) (*protoHandshake, error)

	MsgReadWriter

	close(err error)
}

func (c *conn) String() string {
	s := c.flags.String()
	if (c.id != discover.NodeID{}) {
		s += " " + c.id.String()
	}
	s += " " + c.fd.RemoteAddr().String()
	return s
}

func (f connFlag) String() string {
	s := ""
	if f&trustedConn != 0 {
		s += "-trusted"
	}
	if f&dynDialedConn != 0 {
		s += "-dyndial"
	}
	if f&staticDialedConn != 0 {
		s += "-staticdial"
	}
	if f&inboundConn != 0 {
		s += "-inbound"
	}
	if s != "" {
		s = s[1:]
	}
	return s
}

func (c *conn) is(f connFlag) bool {
	return c.flags&f != 0
}

func (srv *Server) Peers() []*Peer {
	var ps []*Peer
	select {

	case srv.peerOp <- func(peers map[discover.NodeID]*Peer) {
		for _, p := range peers {
			ps = append(ps, p)
		}
	}:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return ps
}

func (srv *Server) PeerCount() int {
	var count int
	select {
	case srv.peerOp <- func(ps map[discover.NodeID]*Peer) { count = len(ps) }:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return count
}

func (srv *Server) AddPeer(node *discover.Node) {
	select {
	case srv.addstatic <- node:
	case <-srv.quit:
	}
}

func (srv *Server) RemovePeer(node *discover.Node) {
	select {
	case srv.removestatic <- node:
	case <-srv.quit:
	}
}

func (srv *Server) SubscribeEvents(ch chan *PeerEvent) event.Subscription {
	return srv.peerFeed.Subscribe(ch)
}

func (srv *Server) Self() *discover.Node {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	if !srv.running {
		return &discover.Node{IP: net.ParseIP("0.0.0.0")}
	}
	return srv.makeSelf(srv.listener, srv.ntab)
}

func (srv *Server) makeSelf(listener net.Listener, ntab discoverTable) *discover.Node {

	if ntab == nil {

		if listener == nil {
			return &discover.Node{IP: net.ParseIP("0.0.0.0"), ID: discover.PubkeyID(&srv.PrivateKey.PublicKey)}
		}

		addr := listener.Addr().(*net.TCPAddr)
		return &discover.Node{
			ID:  discover.PubkeyID(&srv.PrivateKey.PublicKey),
			IP:  addr.IP,
			TCP: uint16(addr.Port),
		}
	}

	return ntab.Self()
}

func (srv *Server) Stop() {
	srv.lock.Lock()
	if !srv.running {
		srv.lock.Unlock()
		return
	}
	srv.eventsSub.Unsubscribe()
	srv.running = false
	if srv.listener != nil {

		srv.listener.Close()
	}
	close(srv.quit)
	srv.lock.Unlock()
	srv.loopWG.Wait()
}

type sharedUDPConn struct {
	*net.UDPConn
	unhandled chan discover.ReadPacket
}

func (s *sharedUDPConn) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	packet, ok := <-s.unhandled
	if !ok {
		return 0, nil, fmt.Errorf("Connection was closed")
	}
	l := len(packet.Data)
	if l > len(b) {
		l = len(b)
	}
	copy(b[:l], packet.Data[:l])
	return l, packet.Addr, nil
}

func (s *sharedUDPConn) Close() error {
	return nil
}

func (srv *Server) Start() (err error) {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if srv.running {
		return errors.New("server already running")
	}
	srv.running = true
	srv.log = srv.Config.Logger
	if srv.log == nil {
		srv.log = log.New()
	}
	srv.log.Info("Starting P2P networking...")

	if srv.PrivateKey == nil {
		return fmt.Errorf("Server.PrivateKey must be set to a non-nil key")
	}
	if srv.newTransport == nil {
		srv.newTransport = newRLPX
	}
	if srv.Dialer == nil {
		srv.Dialer = TCPDialer{&net.Dialer{Timeout: defaultDialTimeout}}
	}
	srv.quit = make(chan struct{})
	srv.addpeer = make(chan *conn)
	srv.delpeer = make(chan peerDrop)
	srv.posthandshake = make(chan *conn)
	srv.addstatic = make(chan *discover.Node)
	srv.removestatic = make(chan *discover.Node)
	srv.peerOp = make(chan peerOpFunc)
	srv.peerOpDone = make(chan struct{})
	srv.events = make(chan *PeerEvent)
	srv.eventsSub = srv.SubscribeEvents(srv.events)

	srv.nodeInfoList = make([]*NodeInfoToSend, 0)

	var (
		conn      *net.UDPConn
		sconn     *sharedUDPConn
		realaddr  *net.UDPAddr
		unhandled chan discover.ReadPacket
	)

	if !srv.NoDiscovery || srv.DiscoveryV5 {
		addr, err := net.ResolveUDPAddr("udp", srv.ListenAddr)
		if err != nil {
			return err
		}
		conn, err = net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		realaddr = conn.LocalAddr().(*net.UDPAddr)
		if srv.NAT != nil {
			if !realaddr.IP.IsLoopback() {
				go nat.Map(srv.NAT, srv.quit, "udp", realaddr.Port, realaddr.Port, "ethereum discovery")
			}

			if ext, err := srv.NAT.ExternalIP(); err == nil {
				realaddr = &net.UDPAddr{IP: ext, Port: realaddr.Port}
			}
		}
	}

	if !srv.NoDiscovery && srv.DiscoveryV5 {
		unhandled = make(chan discover.ReadPacket, 100)
		sconn = &sharedUDPConn{conn, unhandled}
	}

	if !srv.NoDiscovery {
		cfg := discover.Config{
			PrivateKey:   srv.PrivateKey,
			AnnounceAddr: realaddr,
			NodeDBPath:   srv.NodeDatabase,
			NetRestrict:  srv.NetRestrict,
			Bootnodes:    srv.BootstrapNodes,
			Unhandled:    unhandled,
		}
		ntab, err := discover.ListenUDP(conn, cfg)
		if err != nil {
			return err
		}
		srv.ntab = ntab
	}

	if srv.DiscoveryV5 {
		var (
			ntab *discv5.Network
			err  error
		)
		if sconn != nil {
			ntab, err = discv5.ListenUDP(srv.PrivateKey, sconn, realaddr, "", srv.NetRestrict)
		} else {
			ntab, err = discv5.ListenUDP(srv.PrivateKey, conn, realaddr, "", srv.NetRestrict)
		}
		if err != nil {
			return err
		}
		if err := ntab.SetFallbackNodes(srv.BootstrapNodesV5); err != nil {
			return err
		}
		srv.DiscV5 = ntab
	}

	dynPeers := srv.maxDialedConns()
	dialer := newDialState(srv.StaticNodes, srv.BootstrapNodes, srv.ntab, dynPeers, srv.NetRestrict)

	srv.ourHandshake = &protoHandshake{Version: baseProtocolVersion, Name: srv.Name, ID: discover.PubkeyID(&srv.PrivateKey.PublicKey)}
	for _, p := range srv.Protocols {
		srv.ourHandshake.Caps = append(srv.ourHandshake.Caps, p.cap())
	}

	if srv.ListenAddr != "" {
		if err := srv.startListening(); err != nil {
			return err
		}
	}
	if srv.NoDial && srv.ListenAddr == "" {
		srv.log.Warn("P2P server will be useless, neither dialing nor listening")
	}

	srv.loopWG.Add(1)
	go srv.run(dialer)
	srv.running = true

	go srv.sendValidatorNodeInfoMessages()

	return nil
}

func (srv *Server) AddChildProtocolCaps(sideProtocols []Protocol) {
	for _, p := range sideProtocols {
		srv.ourHandshake.Caps = append(srv.ourHandshake.Caps, p.cap())
	}
}

func (srv *Server) startListening() error {

	listener, err := net.Listen("tcp", srv.ListenAddr)
	if err != nil {
		return err
	}
	laddr := listener.Addr().(*net.TCPAddr)
	srv.ListenAddr = laddr.String()
	srv.listener = listener
	srv.loopWG.Add(1)
	go srv.listenLoop()

	if !laddr.IP.IsLoopback() && srv.NAT != nil {
		srv.loopWG.Add(1)
		go func() {
			nat.Map(srv.NAT, srv.quit, "tcp", laddr.Port, laddr.Port, "ethereum p2p")
			srv.loopWG.Done()
		}()
	}
	return nil
}

type dialer interface {
	newTasks(running int, peers map[discover.NodeID]*Peer, now time.Time) []task
	taskDone(task, time.Time)
	addStatic(*discover.Node)
	removeStatic(*discover.Node)
}

func (srv *Server) run(dialstate dialer) {
	defer srv.loopWG.Done()
	var (
		peers        = make(map[discover.NodeID]*Peer)
		inboundCount = 0
		trusted      = make(map[discover.NodeID]bool, len(srv.TrustedNodes))
		taskdone     = make(chan task, maxActiveDialTasks)
		runningTasks []task
		queuedTasks  []task
	)

	for _, n := range srv.TrustedNodes {
		trusted[n.ID] = true
	}

	delTask := func(t task) {
		for i := range runningTasks {
			if runningTasks[i] == t {
				runningTasks = append(runningTasks[:i], runningTasks[i+1:]...)
				break
			}
		}
	}

	startTasks := func(ts []task) (rest []task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			srv.log.Trace("New dial task", "task", t)
			go func() { t.Do(srv); taskdone <- t }()
			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}
	scheduleTasks := func() {

		queuedTasks = append(queuedTasks[:0], startTasks(queuedTasks)...)

		if len(runningTasks) < maxActiveDialTasks {
			nt := dialstate.newTasks(len(runningTasks)+len(queuedTasks), peers, time.Now())
			queuedTasks = append(queuedTasks, startTasks(nt)...)
		}
	}

running:
	for {
		scheduleTasks()

		select {
		case <-srv.quit:

			break running
		case n := <-srv.addstatic:

			srv.log.Debug("Adding static node", "node", n)
			dialstate.addStatic(n)
		case n := <-srv.removestatic:

			srv.log.Debug("Removing static node", "node", n)
			dialstate.removeStatic(n)
			if p, ok := peers[n.ID]; ok {
				p.Disconnect(DiscRequested)
			}
		case op := <-srv.peerOp:

			op(peers)
			srv.peerOpDone <- struct{}{}
		case t := <-taskdone:

			srv.log.Trace("Dial task done", "task", t)
			dialstate.taskDone(t, time.Now())
			delTask(t)
		case c := <-srv.posthandshake:

			if trusted[c.id] {

				c.flags |= trustedConn
			}

			select {
			case c.cont <- srv.encHandshakeChecks(peers, inboundCount, c):
			case <-srv.quit:
				break running
			}
		case c := <-srv.addpeer:

			err := srv.protoHandshakeChecks(peers, inboundCount, c)
			if err == nil {

				p := newPeer(c, srv.Protocols)

				if srv.EnableMsgEvents {
					p.events = &srv.peerFeed
				}
				name := truncateName(c.name)
				srv.log.Debug("Adding p2p peer", "name", name, "addr", c.fd.RemoteAddr(), "peers", len(peers)+1)
				go srv.runPeer(p)
				peers[c.id] = p
				if p.Inbound() {
					inboundCount++
				}

				srv.validatorAddPeer(p)
			}

			select {
			case c.cont <- err:
			case <-srv.quit:
				break running
			}
		case pd := <-srv.delpeer:

			d := common.PrettyDuration(mclock.Now() - pd.created)
			pd.log.Debug("Removing p2p peer", "duration", d, "peers", len(peers)-1, "req", pd.requested, "err", pd.err)
			delete(peers, pd.ID())
			if pd.Inbound() {
				inboundCount--
			}
			srv.validatorDelPeer(pd.ID())

		case evt := <-srv.events:
			log.Debugf("peer events received: %v", evt)
			switch evt.Type {

			case PeerEventTypeRefreshValidator:
				var valNodeInfo P2PValidatorNodeInfo
				if err := rlp.DecodeBytes([]byte(evt.Protocol), &valNodeInfo); err != nil {
					log.Debugf("rlp decode valNodeInfo failed with %v", err)
				}

				peerArr := make([]*Peer, 0)
				for _, p := range peers {
					peerArr = append(peerArr, p)
				}

				if err := srv.validatorAdd(valNodeInfo, peerArr, dialstate); err != nil {
					log.Debugf("add valNodeInfo to local failed with %v", err)
				}

				log.Debugf("Got refresh validation node infomation from validation %v", valNodeInfo.Validator.Address.String())

			case PeerEventTypeRemoveValidator:
				var valNodeInfo P2PValidatorNodeInfo
				if err := rlp.DecodeBytes([]byte(evt.Protocol), &valNodeInfo); err != nil {
					log.Debugf("rlp decode valNodeInfo failed with %v", err)
				}

				peerArr := make([]*Peer, 0)
				for _, p := range peers {
					peerArr = append(peerArr, p)
				}

				if err := srv.validatorRemove(valNodeInfo, peerArr, dialstate); err != nil {
					log.Debugf("remove valNodeInfo from local failed with %v", err)
				}

				log.Debugf("Got remove validation node infomation from validation %v", valNodeInfo.Validator.Address.String())

			}
		}
	}

	srv.log.Trace("P2P networking is spinning down")

	if srv.ntab != nil {
		srv.ntab.Close()
	}
	if srv.DiscV5 != nil {
		srv.DiscV5.Close()
	}

	for _, p := range peers {
		p.Disconnect(DiscQuitting)
	}

	for len(peers) > 0 {
		p := <-srv.delpeer
		p.log.Trace("<-delpeer (spindown)", "remainingTasks", len(runningTasks))
		delete(peers, p.ID())
	}
}

func (srv *Server) protoHandshakeChecks(peers map[discover.NodeID]*Peer, inboundCount int, c *conn) error {

	if len(srv.Protocols) > 0 && countMatchingProtocols(srv.Protocols, c.caps) == 0 {
		return DiscUselessPeer
	}

	return srv.encHandshakeChecks(peers, inboundCount, c)
}

func (srv *Server) encHandshakeChecks(peers map[discover.NodeID]*Peer, inboundCount int, c *conn) error {
	switch {
	case !c.is(trustedConn|staticDialedConn) && len(peers) >= srv.MaxPeers:
		return DiscTooManyPeers
	case !c.is(trustedConn) && c.is(inboundConn) && inboundCount >= srv.maxInboundConns():
		return DiscTooManyPeers
	case peers[c.id] != nil:
		return DiscAlreadyConnected
	case c.id == srv.Self().ID:
		return DiscSelf
	default:
		return nil
	}
}

func (srv *Server) maxInboundConns() int {
	return srv.MaxPeers - srv.maxDialedConns()
}

func (srv *Server) maxDialedConns() int {
	if srv.NoDiscovery || srv.NoDial {
		return 0
	}
	r := srv.DialRatio
	if r == 0 {
		r = defaultDialRatio
	}
	return srv.MaxPeers / r
}

type tempError interface {
	Temporary() bool
}

func (srv *Server) listenLoop() {
	defer srv.loopWG.Done()

	tokens := defaultMaxPendingPeers
	if srv.MaxPendingPeers > 0 {
		tokens = srv.MaxPendingPeers
	}
	slots := make(chan struct{}, tokens)
	for i := 0; i < tokens; i++ {
		slots <- struct{}{}
	}

	for {

		<-slots

		var (
			fd  net.Conn
			err error
		)
		for {
			fd, err = srv.listener.Accept()
			if tempErr, ok := err.(tempError); ok && tempErr.Temporary() {
				srv.log.Debug("Temporary read error", "err", err)
				continue
			} else if err != nil {
				srv.log.Debug("Read error", "err", err)
				return
			}
			break
		}

		if srv.NetRestrict != nil {
			if tcp, ok := fd.RemoteAddr().(*net.TCPAddr); ok && !srv.NetRestrict.Contains(tcp.IP) {
				srv.log.Debug("Rejected conn (not whitelisted in NetRestrict)", "addr", fd.RemoteAddr())
				fd.Close()
				slots <- struct{}{}
				continue
			}
		}

		fd = newMeteredConn(fd, true)
		srv.log.Trace("Accepted connection", "addr", fd.RemoteAddr())
		go func() {
			srv.SetupConn(fd, inboundConn, nil)
			slots <- struct{}{}
		}()
	}
}

func (srv *Server) SetupConn(fd net.Conn, flags connFlag, dialDest *discover.Node) error {
	self := srv.Self()
	if self == nil {
		return errors.New("shutdown")
	}
	c := &conn{fd: fd, transport: srv.newTransport(fd), flags: flags, cont: make(chan error)}
	err := srv.setupConn(c, flags, dialDest)
	if err != nil {
		c.close(err)
		srv.log.Trace("Setting up connection failed", "id", c.id, "err", err)
	}
	return err
}

func (srv *Server) setupConn(c *conn, flags connFlag, dialDest *discover.Node) error {

	srv.lock.Lock()
	running := srv.running
	srv.lock.Unlock()
	if !running {
		return errServerStopped
	}

	var err error
	if c.id, err = c.doEncHandshake(srv.PrivateKey, dialDest); err != nil {
		srv.log.Trace("Failed RLPx handshake", "addr", c.fd.RemoteAddr(), "conn", c.flags, "err", err)
		return err
	}
	clog := srv.log.New("id", c.id, "addr", c.fd.RemoteAddr(), "conn", c.flags)

	if dialDest != nil && c.id != dialDest.ID {
		clog.Trace("Dialed identity mismatch", "want", c, dialDest.ID)
		return DiscUnexpectedIdentity
	}
	err = srv.checkpoint(c, srv.posthandshake)
	if err != nil {
		clog.Trace("Rejected peer before protocol handshake", "err", err)
		return err
	}

	phs, err := c.doProtoHandshake(srv.ourHandshake)
	if err != nil {
		clog.Trace("Failed proto handshake", "err", err)
		return err
	}
	if phs.ID != c.id {
		clog.Trace("Wrong devp2p handshake identity", "err", phs.ID)
		return DiscUnexpectedIdentity
	}
	c.caps, c.name = phs.Caps, phs.Name
	err = srv.checkpoint(c, srv.addpeer)
	if err != nil {
		clog.Trace("Rejected peer", "err", err)
		return err
	}

	clog.Trace("connection set up", "inbound", dialDest == nil)
	return nil
}

func truncateName(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

func (srv *Server) checkpoint(c *conn, stage chan<- *conn) error {
	select {
	case stage <- c:
	case <-srv.quit:
		return errServerStopped
	}
	select {
	case err := <-c.cont:
		return err
	case <-srv.quit:
		return errServerStopped
	}
}

func (srv *Server) runPeer(p *Peer) {
	if srv.newPeerHook != nil {
		srv.newPeerHook(p)
	}

	srv.peerFeed.Send(&PeerEvent{
		Type: PeerEventTypeAdd,
		Peer: p.ID(),
	})

	p.srvProtocols = &srv.Protocols

	remoteRequested, err := p.run()

	srv.peerFeed.Send(&PeerEvent{
		Type:  PeerEventTypeDrop,
		Peer:  p.ID(),
		Error: err.Error(),
	})

	srv.delpeer <- peerDrop{p, err, remoteRequested}
}

type NodeInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Enode string `json:"enode"`
	IP    string `json:"ip"`
	Ports struct {
		Discovery int `json:"discovery"`
		Listener  int `json:"listener"`
	} `json:"ports"`
	ListenAddr string                 `json:"listenAddr"`
	Protocols  map[string]interface{} `json:"protocols"`
}

func (srv *Server) NodeInfo() *NodeInfo {
	node := srv.Self()

	info := &NodeInfo{
		Name:       srv.Name,
		Enode:      node.String(),
		ID:         node.ID.String(),
		IP:         node.IP.String(),
		ListenAddr: srv.ListenAddr,
		Protocols:  make(map[string]interface{}),
	}
	info.Ports.Discovery = int(node.UDP)
	info.Ports.Listener = int(node.TCP)

	for _, proto := range srv.Protocols {
		if _, ok := info.Protocols[proto.Name]; !ok {
			nodeInfo := interface{}("unknown")
			if query := proto.NodeInfo; query != nil {
				nodeInfo = proto.NodeInfo()
			}
			info.Protocols[proto.Name] = nodeInfo
		}
	}
	return info
}

func (srv *Server) PeersInfo() []*PeerInfo {

	infos := make([]*PeerInfo, 0, srv.PeerCount())
	for _, peer := range srv.Peers() {
		if peer != nil {
			infos = append(infos, peer.Info())
		}
	}

	for i := 0; i < len(infos); i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[i].ID > infos[j].ID {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}
	return infos
}

func (srv *Server) BroadcastMsg(msgCode uint64, data interface{}) {
	peers := srv.Peers()
	for _, p := range peers {
		Send(p.rw, BroadcastNewSideChainMsg, data)
	}
}

func (srv *Server) AddLocalValidator(chainId string, address common.Address) {

	log.Debug("AddLocalValidator")

	validator := P2PValidator{
		ChainId: chainId,
		Address: address,
	}

	for i := 0; i < len(srv.LocalValidators); i++ {
		if validator == srv.LocalValidators[i] {
			return
		}
	}

	srv.LocalValidators = append(srv.LocalValidators, validator)

	srv.broadcastRefreshValidatorNodeInfo(&P2PValidatorNodeInfo{
		Node:      *srv.Self(),
		TimeStamp: time.Now(),
		Validator: validator,
		Original:  true,
	}, nil)
}

func (srv *Server) RemoveLocalValidator(chainId string, address common.Address) {

	log.Debug("RemoveLocalValidator")

	validator := P2PValidator{
		ChainId: chainId,
		Address: address,
	}

	idx := -1
	for i := 0; i < len(srv.LocalValidators); i++ {
		if validator == srv.LocalValidators[i] {
			idx = i
			break
		}
	}

	if idx < 0 {
		return
	}

	srv.LocalValidators = append(srv.LocalValidators[:idx], srv.LocalValidators[idx+1:]...)

	srv.broadcastRemoveValidatorNodeInfo(&P2PValidatorNodeInfo{
		Node:      *srv.Self(),
		TimeStamp: time.Now(),
		Validator: validator,
		Original:  true,
	}, nil)
}

func (srv *Server) validatorAdd(valNodeInfo P2PValidatorNodeInfo, peers []*Peer, dialstate dialer) error {

	log.Debug("validatorAdd")

	validator := valNodeInfo.Validator

	if srv.Self().ID == valNodeInfo.Node.ID {
		return nil
	}

	if nodeInfo, ok := srv.Validators[validator]; ok {
		con1 := valNodeInfo.Node.ID == nodeInfo.Node.ID
		con2 := valNodeInfo.Node.IP.String() == nodeInfo.Node.IP.String()
		log.Debugf("con1: %v, con2: %v", con1, con2)
		if con1 && con2 {
			log.Debug("validator found, not add")
			return nil
		}
	}

	log.Debug("validator not found")
	srv.Validators[validator] = &valNodeInfo
	inSameChain := false

	for i := 0; i < len(srv.LocalValidators); i++ {
		if validator.ChainId == srv.LocalValidators[i].ChainId {
			inSameChain = true
			break
		}
	}

	notPeer := true
	for _, p := range peers {
		if p.ID() == valNodeInfo.Node.ID {
			notPeer = false
			break
		}
	}

	if inSameChain && notPeer {
		dialstate.addStatic(&valNodeInfo.Node)
	}

	return nil
}

func (srv *Server) validatorRemove(valNodeInfo P2PValidatorNodeInfo, peers []*Peer, dialstate dialer) error {

	log.Debug("validatorRemove")

	if srv.Self().ID == valNodeInfo.Node.ID {
		return nil
	}

	validator := valNodeInfo.Validator

	if _, ok := srv.Validators[validator]; !ok {
		return nil
	}

	delete(srv.Validators, validator)

	inSameChain := 0

	for i := 0; i < len(srv.LocalValidators); i++ {
		if validator.ChainId == srv.LocalValidators[i].ChainId {
			inSameChain++
		}
	}

	shouldRemove := false

	if inSameChain == 1 {
		for _, p := range peers {
			if p.ID() == valNodeInfo.Node.ID {

				shouldRemove = true
				break
			}
		}
	}

	if shouldRemove {
		dialstate.removeStatic(&valNodeInfo.Node)
	}

	return nil
}

func (srv *Server) validatorAddPeer(peer *Peer) error {

	log.Debug("validatorAddPeer")

	sendList := make([]*NodeInfoToSend, 0)

	var err error = nil
	for _, validatorNodeInfo := range srv.Validators {

		if peer.ID() == validatorNodeInfo.Node.ID {

			validatorNodeInfo.Node.IP = peer.RemoteAddr().(*net.TCPAddr).IP
			continue
		}

		sendList = append(sendList, &NodeInfoToSend{
			valNodeInfo: validatorNodeInfo,
			action:      RefreshValidatorNodeInfoMsg,
			p:           peer,
		})
	}

	node := *srv.Self()
	for i := 0; i < len(srv.LocalValidators); i++ {

		sendList = append(sendList, &NodeInfoToSend{
			valNodeInfo: &P2PValidatorNodeInfo{
				Node:      node,
				TimeStamp: time.Now(),
				Validator: srv.LocalValidators[i],
				Original:  true,
			},
			action: RefreshValidatorNodeInfoMsg,
			p:      peer,
		})
	}

	srv.addNodeInfoToSend(sendList)

	return err
}

func (srv *Server) validatorDelPeer(nodeId discover.NodeID) error {

	log.Debug("validatorDelPeer")

	srv.nodeInfoLock.Lock()
	defer srv.nodeInfoLock.Unlock()

	tailIndex := 0
	for i := 0; i < len(srv.nodeInfoList); i++ {

		nodeInfo := srv.nodeInfoList[i]
		if nodeInfo.valNodeInfo.Node.ID != nodeId {
			if i != tailIndex {
				srv.nodeInfoList[tailIndex] = nodeInfo
			}
			tailIndex++
		}
	}

	removedCount := len(srv.nodeInfoList) - 1 - tailIndex

	srv.nodeInfoList = srv.nodeInfoList[:tailIndex]

	log.Debugf("removed %v node info to send to %v", removedCount, nodeId)

	return nil
}

func (srv *Server) broadcastRefreshValidatorNodeInfo(data *P2PValidatorNodeInfo, peers []*Peer) {

	log.Debug("broadcastRefreshValidatorNodeInfo")
	if peers == nil {
		peers = srv.Peers()
	}

	sendList := make([]*NodeInfoToSend, 0)
	for _, p := range peers {

		sendList = append(sendList, &NodeInfoToSend{
			valNodeInfo: data,
			action:      RefreshValidatorNodeInfoMsg,
			p:           p,
		})

	}

	srv.addNodeInfoToSend(sendList)
}

func (srv *Server) broadcastRemoveValidatorNodeInfo(data interface{}, peers []*Peer) {

	log.Debug("broadcastRemoveValidatorNodeInfo")
	if peers == nil {
		peers = srv.Peers()
	}

	sendList := make([]*NodeInfoToSend, 0)
	for _, p := range peers {

		sendList = append(sendList, &NodeInfoToSend{
			valNodeInfo: data.(*P2PValidatorNodeInfo),
			action:      RemoveValidatorNodeInfoMsg,
			p:           p,
		})

	}

	srv.addNodeInfoToSend(sendList)
}

func (srv *Server) addNodeInfoToSend(sendList []*NodeInfoToSend) {

	srv.nodeInfoLock.Lock()
	defer srv.nodeInfoLock.Unlock()

	srv.nodeInfoList = append(srv.nodeInfoList, sendList...)
}

func (srv *Server) sendValidatorNodeInfoMessages() {

	sleepDuration := 100 * time.Millisecond

	for srv.running {

		if len(srv.nodeInfoList) > 0 {
			srv.nodeInfoLock.Lock()

			nodeInfo := srv.nodeInfoList[0]
			srv.nodeInfoList = srv.nodeInfoList[1:]

			srv.nodeInfoLock.Unlock()

			if nodeInfo != nil &&
				nodeInfo.valNodeInfo != nil &&
				nodeInfo.p != nil &&
				nodeInfo.p.rw != nil &&
				nodeInfo.p.rw.fd != nil {

				Send(nodeInfo.p.rw, nodeInfo.action, nodeInfo.valNodeInfo)

				log.Debugf("send node info (%v, %v) to %v",
					nodeInfo.valNodeInfo.Validator.Address.String(), nodeInfo.valNodeInfo.Node.ID,
					nodeInfo.p.ID())
			}
		}

		time.Sleep(sleepDuration)
	}
}
