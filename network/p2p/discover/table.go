package discover

import (
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	mrand "math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/network/p2p/netutil"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/crypto"
)

const (
	alpha           = 3
	bucketSize      = 16
	maxReplacements = 10

	hashBits          = len(common.Hash{}) * 8
	nBuckets          = hashBits / 15
	bucketMinDistance = hashBits - nBuckets

	bucketIPLimit, bucketSubnet = 2, 24
	tableIPLimit, tableSubnet   = 10, 24

	maxBondingPingPongs = 16
	maxFindnodeFailures = 5

	refreshInterval    = 30 * time.Minute
	revalidateInterval = 10 * time.Second
	copyNodesInterval  = 30 * time.Second
	seedMinTableTime   = 5 * time.Minute
	seedCount          = 30
	seedMaxAge         = 5 * 24 * time.Hour
)

type Table struct {
	mutex   sync.Mutex
	buckets [nBuckets]*bucket
	nursery []*Node
	rand    *mrand.Rand
	ips     netutil.DistinctNetSet

	db         *nodeDB
	refreshReq chan chan struct{}
	initDone   chan struct{}
	closeReq   chan struct{}
	closed     chan struct{}

	bondmu    sync.Mutex
	bonding   map[NodeID]*bondproc
	bondslots chan struct{}

	nodeAddedHook func(*Node)

	net  transport
	self *Node
}

type bondproc struct {
	err  error
	n    *Node
	done chan struct{}
}

type transport interface {
	ping(NodeID, *net.UDPAddr) error
	waitping(NodeID) error
	findnode(toid NodeID, addr *net.UDPAddr, target NodeID) ([]*Node, error)
	close()
}

type bucket struct {
	entries      []*Node
	replacements []*Node
	ips          netutil.DistinctNetSet
}

func newTable(t transport, ourID NodeID, ourAddr *net.UDPAddr, nodeDBPath string, bootnodes []*Node) (*Table, error) {

	db, err := newNodeDB(nodeDBPath, Version, ourID)
	if err != nil {
		return nil, err
	}
	tab := &Table{
		net:        t,
		db:         db,
		self:       NewNode(ourID, ourAddr.IP, uint16(ourAddr.Port), uint16(ourAddr.Port)),
		bonding:    make(map[NodeID]*bondproc),
		bondslots:  make(chan struct{}, maxBondingPingPongs),
		refreshReq: make(chan chan struct{}),
		initDone:   make(chan struct{}),
		closeReq:   make(chan struct{}),
		closed:     make(chan struct{}),
		rand:       mrand.New(mrand.NewSource(0)),
		ips:        netutil.DistinctNetSet{Subnet: tableSubnet, Limit: tableIPLimit},
	}
	if err := tab.setFallbackNodes(bootnodes); err != nil {
		return nil, err
	}
	for i := 0; i < cap(tab.bondslots); i++ {
		tab.bondslots <- struct{}{}
	}
	for i := range tab.buckets {
		tab.buckets[i] = &bucket{
			ips: netutil.DistinctNetSet{Subnet: bucketSubnet, Limit: bucketIPLimit},
		}
	}
	tab.seedRand()
	tab.loadSeedNodes(false)

	tab.db.ensureExpirer()
	go tab.loop()
	return tab, nil
}

func (tab *Table) seedRand() {
	var b [8]byte
	crand.Read(b[:])

	tab.mutex.Lock()
	tab.rand.Seed(int64(binary.BigEndian.Uint64(b[:])))
	tab.mutex.Unlock()
}

func (tab *Table) Self() *Node {
	return tab.self
}

func (tab *Table) ReadRandomNodes(buf []*Node) (n int) {
	if !tab.isInitDone() {
		return 0
	}
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	var buckets [][]*Node
	for _, b := range tab.buckets {
		if len(b.entries) > 0 {
			buckets = append(buckets, b.entries[:])
		}
	}
	if len(buckets) == 0 {
		return 0
	}

	for i := len(buckets) - 1; i > 0; i-- {
		j := tab.rand.Intn(len(buckets))
		buckets[i], buckets[j] = buckets[j], buckets[i]
	}

	var i, j int
	for ; i < len(buf); i, j = i+1, (j+1)%len(buckets) {
		b := buckets[j]
		buf[i] = &(*b[0])
		buckets[j] = b[1:]
		if len(b) == 1 {
			buckets = append(buckets[:j], buckets[j+1:]...)
		}
		if len(buckets) == 0 {
			break
		}
	}
	return i + 1
}

func (tab *Table) Close() {
	select {
	case <-tab.closed:

	case tab.closeReq <- struct{}{}:
		<-tab.closed
	}
}

func (tab *Table) setFallbackNodes(nodes []*Node) error {
	for _, n := range nodes {
		if err := n.validateComplete(); err != nil {
			return fmt.Errorf("bad bootstrap/fallback node %q (%v)", n, err)
		}
	}
	tab.nursery = make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		cpy := *n

		cpy.sha = crypto.Keccak256Hash(n.ID[:])
		tab.nursery = append(tab.nursery, &cpy)
	}
	return nil
}

func (tab *Table) isInitDone() bool {
	select {
	case <-tab.initDone:
		return true
	default:
		return false
	}
}

func (tab *Table) Resolve(targetID NodeID) *Node {

	hash := crypto.Keccak256Hash(targetID[:])
	tab.mutex.Lock()
	cl := tab.closest(hash, 1)
	tab.mutex.Unlock()
	if len(cl.entries) > 0 && cl.entries[0].ID == targetID {
		return cl.entries[0]
	}

	result := tab.Lookup(targetID)
	for _, n := range result {
		if n.ID == targetID {
			return n
		}
	}
	return nil
}

func (tab *Table) Lookup(targetID NodeID) []*Node {
	return tab.lookup(targetID, true)
}

func (tab *Table) lookup(targetID NodeID, refreshIfEmpty bool) []*Node {
	var (
		target         = crypto.Keccak256Hash(targetID[:])
		asked          = make(map[NodeID]bool)
		seen           = make(map[NodeID]bool)
		reply          = make(chan []*Node, alpha)
		pendingQueries = 0
		result         *nodesByDistance
	)

	asked[tab.self.ID] = true

	for {
		tab.mutex.Lock()

		result = tab.closest(target, bucketSize)
		tab.mutex.Unlock()
		if len(result.entries) > 0 || !refreshIfEmpty {
			break
		}

		<-tab.refresh()
		refreshIfEmpty = false
	}

	for {

		for i := 0; i < len(result.entries) && pendingQueries < alpha; i++ {
			n := result.entries[i]
			if !asked[n.ID] {
				asked[n.ID] = true
				pendingQueries++
				go func() {

					r, err := tab.net.findnode(n.ID, n.addr(), targetID)
					if err != nil {

						fails := tab.db.findFails(n.ID) + 1
						tab.db.updateFindFails(n.ID, fails)
						log.Trace("Bumping findnode failure counter", "id", n.ID, "failcount", fails)

						if fails >= maxFindnodeFailures {
							log.Trace("Too many findnode failures, dropping", "id", n.ID, "failcount", fails)
							tab.delete(n)
						}
					}
					reply <- tab.bondall(r)
				}()
			}
		}
		if pendingQueries == 0 {

			break
		}

		for _, n := range <-reply {
			if n != nil && !seen[n.ID] {
				seen[n.ID] = true
				result.push(n, bucketSize)
			}
		}
		pendingQueries--
	}
	return result.entries
}

func (tab *Table) refresh() <-chan struct{} {
	done := make(chan struct{})
	select {
	case tab.refreshReq <- done:
	case <-tab.closed:
		close(done)
	}
	return done
}

func (tab *Table) loop() {
	var (
		revalidate     = time.NewTimer(tab.nextRevalidateTime())
		refresh        = time.NewTicker(refreshInterval)
		copyNodes      = time.NewTicker(copyNodesInterval)
		revalidateDone = make(chan struct{})
		refreshDone    = make(chan struct{})
		waiting        = []chan struct{}{tab.initDone}
	)
	defer refresh.Stop()
	defer revalidate.Stop()
	defer copyNodes.Stop()

	go tab.doRefresh(refreshDone)

loop:
	for {
		select {
		case <-refresh.C:
			tab.seedRand()
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case req := <-tab.refreshReq:
			waiting = append(waiting, req)
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case <-refreshDone:
			for _, ch := range waiting {
				close(ch)
			}
			waiting, refreshDone = nil, nil
		case <-revalidate.C:
			go tab.doRevalidate(revalidateDone)
		case <-revalidateDone:
			revalidate.Reset(tab.nextRevalidateTime())
		case <-copyNodes.C:
			go tab.copyBondedNodes()
		case <-tab.closeReq:
			break loop
		}
	}

	if tab.net != nil {
		tab.net.close()
	}
	if refreshDone != nil {
		<-refreshDone
	}
	for _, ch := range waiting {
		close(ch)
	}
	tab.db.close()
	close(tab.closed)
}

func (tab *Table) doRefresh(done chan struct{}) {
	defer close(done)

	tab.loadSeedNodes(true)

	tab.lookup(tab.self.ID, false)

	for i := 0; i < 3; i++ {
		var target NodeID
		crand.Read(target[:])
		tab.lookup(target, false)
	}
}

func (tab *Table) loadSeedNodes(bond bool) {
	seeds := tab.db.querySeeds(seedCount, seedMaxAge)
	seeds = append(seeds, tab.nursery...)
	if bond {
		seeds = tab.bondall(seeds)
	}
	for i := range seeds {
		seed := seeds[i]
		age := log.Lazy{Fn: func() interface{} { return time.Since(tab.db.bondTime(seed.ID)) }}
		log.Debug("Found seed node in database", "id", seed.ID, "addr", seed.addr(), "age", age)
		tab.add(seed)
	}
}

func (tab *Table) doRevalidate(done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	last, bi := tab.nodeToRevalidate()
	if last == nil {

		return
	}

	err := tab.ping(last.ID, last.addr())

	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	b := tab.buckets[bi]
	if err == nil {

		log.Debug("Revalidated node", "b", bi, "id", last.ID)
		b.bump(last)
		return
	}

	if r := tab.replace(b, last); r != nil {
		log.Debug("Replaced dead node", "b", bi, "id", last.ID, "ip", last.IP, "r", r.ID, "rip", r.IP)
	} else {
		log.Debug("Removed dead node", "b", bi, "id", last.ID, "ip", last.IP)
	}
}

func (tab *Table) nodeToRevalidate() (n *Node, bi int) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, bi = range tab.rand.Perm(len(tab.buckets)) {
		b := tab.buckets[bi]
		if len(b.entries) > 0 {
			last := b.entries[len(b.entries)-1]
			return last, bi
		}
	}
	return nil, 0
}

func (tab *Table) nextRevalidateTime() time.Duration {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	return time.Duration(tab.rand.Int63n(int64(revalidateInterval)))
}

func (tab *Table) copyBondedNodes() {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	now := time.Now()
	for _, b := range tab.buckets {
		for _, n := range b.entries {
			if now.Sub(n.addedAt) >= seedMinTableTime {
				tab.db.updateNode(n)
			}
		}
	}
}

func (tab *Table) closest(target common.Hash, nresults int) *nodesByDistance {

	close := &nodesByDistance{target: target}
	for _, b := range tab.buckets {
		for _, n := range b.entries {
			close.push(n, nresults)
		}
	}
	return close
}

func (tab *Table) len() (n int) {
	for _, b := range tab.buckets {
		n += len(b.entries)
	}
	return n
}

func (tab *Table) bondall(nodes []*Node) (result []*Node) {
	rc := make(chan *Node, len(nodes))
	for i := range nodes {
		go func(n *Node) {
			nn, _ := tab.bond(false, n.ID, n.addr(), n.TCP)
			rc <- nn
		}(nodes[i])
	}
	for range nodes {
		if n := <-rc; n != nil {
			result = append(result, n)
		}
	}
	return result
}

func (tab *Table) bond(pinged bool, id NodeID, addr *net.UDPAddr, tcpPort uint16) (*Node, error) {
	if id == tab.self.ID {
		return nil, errors.New("is self")
	}
	if pinged && !tab.isInitDone() {
		return nil, errors.New("still initializing")
	}

	node, fails := tab.db.node(id), tab.db.findFails(id)
	age := time.Since(tab.db.bondTime(id))
	var result error
	if fails > 0 || age > nodeDBNodeExpiration {
		log.Trace("Starting bonding ping/pong", "id", id, "known", node != nil, "failcount", fails, "age", age)

		tab.bondmu.Lock()
		w := tab.bonding[id]
		if w != nil {

			tab.bondmu.Unlock()
			<-w.done
		} else {

			w = &bondproc{done: make(chan struct{})}
			tab.bonding[id] = w
			tab.bondmu.Unlock()

			tab.pingpong(w, pinged, id, addr, tcpPort)

			tab.bondmu.Lock()
			delete(tab.bonding, id)
			tab.bondmu.Unlock()
		}

		result = w.err
		if result == nil {
			node = w.n
		}
	}

	if node != nil {
		tab.add(node)
		tab.db.updateFindFails(id, 0)
	}
	return node, result
}

func (tab *Table) pingpong(w *bondproc, pinged bool, id NodeID, addr *net.UDPAddr, tcpPort uint16) {

	<-tab.bondslots
	defer func() { tab.bondslots <- struct{}{} }()

	if w.err = tab.ping(id, addr); w.err != nil {
		close(w.done)
		return
	}
	if !pinged {

		tab.net.waitping(id)
	}

	w.n = NewNode(id, addr.IP, uint16(addr.Port), tcpPort)
	close(w.done)
}

func (tab *Table) ping(id NodeID, addr *net.UDPAddr) error {
	tab.db.updateLastPing(id, time.Now())
	if err := tab.net.ping(id, addr); err != nil {
		return err
	}
	tab.db.updateBondTime(id, time.Now())
	return nil
}

func (tab *Table) bucket(sha common.Hash) *bucket {
	d := logdist(tab.self.sha, sha)
	if d <= bucketMinDistance {
		return tab.buckets[0]
	}
	return tab.buckets[d-bucketMinDistance-1]
}

func (tab *Table) add(new *Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	b := tab.bucket(new.sha)
	if !tab.bumpOrAdd(b, new) {

		tab.addReplacement(b, new)
	}
}

func (tab *Table) stuff(nodes []*Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, n := range nodes {
		if n.ID == tab.self.ID {
			continue
		}
		b := tab.bucket(n.sha)
		if len(b.entries) < bucketSize {
			tab.bumpOrAdd(b, n)
		}
	}
}

func (tab *Table) delete(node *Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	tab.deleteInBucket(tab.bucket(node.sha), node)
}

func (tab *Table) addIP(b *bucket, ip net.IP) bool {
	if netutil.IsLAN(ip) {
		return true
	}
	if !tab.ips.Add(ip) {
		log.Debug("IP exceeds table limit", "ip", ip)
		return false
	}
	if !b.ips.Add(ip) {
		log.Debug("IP exceeds bucket limit", "ip", ip)
		tab.ips.Remove(ip)
		return false
	}
	return true
}

func (tab *Table) removeIP(b *bucket, ip net.IP) {
	if netutil.IsLAN(ip) {
		return
	}
	tab.ips.Remove(ip)
	b.ips.Remove(ip)
}

func (tab *Table) addReplacement(b *bucket, n *Node) {
	for _, e := range b.replacements {
		if e.ID == n.ID {
			return
		}
	}
	if !tab.addIP(b, n.IP) {
		return
	}
	var removed *Node
	b.replacements, removed = pushNode(b.replacements, n, maxReplacements)
	if removed != nil {
		tab.removeIP(b, removed.IP)
	}
}

func (tab *Table) replace(b *bucket, last *Node) *Node {
	if len(b.entries) == 0 || b.entries[len(b.entries)-1].ID != last.ID {

		return nil
	}

	if len(b.replacements) == 0 {
		tab.deleteInBucket(b, last)
		return nil
	}
	r := b.replacements[tab.rand.Intn(len(b.replacements))]
	b.replacements = deleteNode(b.replacements, r)
	b.entries[len(b.entries)-1] = r
	tab.removeIP(b, last.IP)
	return r
}

func (b *bucket) bump(n *Node) bool {
	for i := range b.entries {
		if b.entries[i].ID == n.ID {

			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}

func (tab *Table) bumpOrAdd(b *bucket, n *Node) bool {
	if b.bump(n) {
		return true
	}
	if len(b.entries) >= bucketSize || !tab.addIP(b, n.IP) {
		return false
	}
	b.entries, _ = pushNode(b.entries, n, bucketSize)
	b.replacements = deleteNode(b.replacements, n)
	n.addedAt = time.Now()
	if tab.nodeAddedHook != nil {
		tab.nodeAddedHook(n)
	}
	return true
}

func (tab *Table) deleteInBucket(b *bucket, n *Node) {
	b.entries = deleteNode(b.entries, n)
	tab.removeIP(b, n.IP)
}

func pushNode(list []*Node, n *Node, max int) ([]*Node, *Node) {
	if len(list) < max {
		list = append(list, nil)
	}
	removed := list[len(list)-1]
	copy(list[1:], list)
	list[0] = n
	return list, removed
}

func deleteNode(list []*Node, n *Node) []*Node {
	for i := range list {
		if list[i].ID == n.ID {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

type nodesByDistance struct {
	entries []*Node
	target  common.Hash
}

func (h *nodesByDistance) push(n *Node, maxElems int) {
	ix := sort.Search(len(h.entries), func(i int) bool {
		return distcmp(h.target, h.entries[i].sha, n.sha) > 0
	})
	if len(h.entries) < maxElems {
		h.entries = append(h.entries, n)
	}
	if ix == len(h.entries) {

	} else {

		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}
