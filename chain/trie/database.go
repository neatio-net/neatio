// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/allegro/bigcache"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/metrics"
	"github.com/neatlab/neatio/utilities/rlp"
)

var (
	memcacheCleanHitMeter   = metrics.NewRegisteredMeter("trie/memcache/clean/hit", nil)
	memcacheCleanMissMeter  = metrics.NewRegisteredMeter("trie/memcache/clean/miss", nil)
	memcacheCleanReadMeter  = metrics.NewRegisteredMeter("trie/memcache/clean/read", nil)
	memcacheCleanWriteMeter = metrics.NewRegisteredMeter("trie/memcache/clean/write", nil)

	memcacheFlushTimeTimer  = metrics.NewRegisteredResettingTimer("trie/memcache/flush/time", nil)
	memcacheFlushNodesMeter = metrics.NewRegisteredMeter("trie/memcache/flush/nodes", nil)
	memcacheFlushSizeMeter  = metrics.NewRegisteredMeter("trie/memcache/flush/size", nil)

	memcacheGCTimeTimer  = metrics.NewRegisteredResettingTimer("trie/memcache/gc/time", nil)
	memcacheGCNodesMeter = metrics.NewRegisteredMeter("trie/memcache/gc/nodes", nil)
	memcacheGCSizeMeter  = metrics.NewRegisteredMeter("trie/memcache/gc/size", nil)

	memcacheCommitTimeTimer  = metrics.NewRegisteredResettingTimer("trie/memcache/commit/time", nil)
	memcacheCommitNodesMeter = metrics.NewRegisteredMeter("trie/memcache/commit/nodes", nil)
	memcacheCommitSizeMeter  = metrics.NewRegisteredMeter("trie/memcache/commit/size", nil)
)

var secureKeyPrefix = []byte("secure-key-")

const secureKeyLength = 11 + 32

type Database struct {
	diskdb neatdb.KeyValueStore

	cleans  *bigcache.BigCache
	dirties map[common.Hash]*cachedNode
	oldest  common.Hash
	newest  common.Hash

	preimages map[common.Hash][]byte
	seckeybuf [secureKeyLength]byte

	gctime  time.Duration
	gcnodes uint64
	gcsize  common.StorageSize

	flushtime  time.Duration
	flushnodes uint64
	flushsize  common.StorageSize

	dirtiesSize   common.StorageSize
	preimagesSize common.StorageSize

	lock sync.RWMutex
}

type rawNode []byte

func (n rawNode) canUnload(uint16, uint16) bool { panic("this should never end up in a live trie") }
func (n rawNode) cache() (hashNode, bool)       { panic("this should never end up in a live trie") }
func (n rawNode) fstring(ind string) string     { panic("this should never end up in a live trie") }

type rawFullNode [17]node

func (n rawFullNode) canUnload(uint16, uint16) bool { panic("this should never end up in a live trie") }
func (n rawFullNode) cache() (hashNode, bool)       { panic("this should never end up in a live trie") }
func (n rawFullNode) fstring(ind string) string     { panic("this should never end up in a live trie") }

func (n rawFullNode) EncodeRLP(w io.Writer) error {
	var nodes [17]node

	for i, side := range n {
		if side != nil {
			nodes[i] = side
		} else {
			nodes[i] = nilValueNode
		}
	}
	return rlp.Encode(w, nodes)
}

type rawShortNode struct {
	Key []byte
	Val node
}

func (n rawShortNode) canUnload(uint16, uint16) bool {
	panic("this should never end up in a live trie")
}
func (n rawShortNode) cache() (hashNode, bool)   { panic("this should never end up in a live trie") }
func (n rawShortNode) fstring(ind string) string { panic("this should never end up in a live trie") }

type cachedNode struct {
	node node
	size uint16

	parents uint32
	sideren map[common.Hash]uint16

	flushPrev common.Hash
	flushNext common.Hash
}

func (n *cachedNode) rlp() []byte {
	if node, ok := n.node.(rawNode); ok {
		return node
	}
	blob, err := rlp.EncodeToBytes(n.node)
	if err != nil {
		panic(err)
	}
	return blob
}

func (n *cachedNode) obj(hash common.Hash) node {
	if node, ok := n.node.(rawNode); ok {
		return mustDecodeNode(hash[:], node)
	}
	return expandNode(hash[:], n.node)
}

func (n *cachedNode) sides() []common.Hash {
	sideren := make([]common.Hash, 0, 16)
	for side := range n.sideren {
		sideren = append(sideren, side)
	}
	if _, ok := n.node.(rawNode); !ok {
		gatherChildren(n.node, &sideren)
	}
	return sideren
}

func gatherChildren(n node, sideren *[]common.Hash) {
	switch n := n.(type) {
	case *rawShortNode:
		gatherChildren(n.Val, sideren)

	case rawFullNode:
		for i := 0; i < 16; i++ {
			gatherChildren(n[i], sideren)
		}
	case hashNode:
		*sideren = append(*sideren, common.BytesToHash(n))

	case valueNode, nil:

	default:
		panic(fmt.Sprintf("unknown node type: %T", n))
	}
}

func simplifyNode(n node) node {
	switch n := n.(type) {
	case *shortNode:

		return &rawShortNode{Key: n.Key, Val: simplifyNode(n.Val)}

	case *fullNode:

		node := rawFullNode(n.Children)
		for i := 0; i < len(node); i++ {
			if node[i] != nil {
				node[i] = simplifyNode(node[i])
			}
		}
		return node

	case valueNode, hashNode, rawNode:
		return n

	default:
		panic(fmt.Sprintf("unknown node type: %T", n))
	}
}

func expandNode(hash hashNode, n node) node {
	switch n := n.(type) {
	case *rawShortNode:

		return &shortNode{
			Key: compactToHex(n.Key),
			Val: expandNode(nil, n.Val),
			flags: nodeFlag{
				hash: hash,
			},
		}

	case rawFullNode:

		node := &fullNode{
			flags: nodeFlag{
				hash: hash,
			},
		}
		for i := 0; i < len(node.Children); i++ {
			if n[i] != nil {
				node.Children[i] = expandNode(nil, n[i])
			}
		}
		return node

	case valueNode, hashNode:
		return n

	default:
		panic(fmt.Sprintf("unknown node type: %T", n))
	}
}

type trienodeHasher struct{}

func (t trienodeHasher) Sum64(key string) uint64 {
	return binary.BigEndian.Uint64([]byte(key))
}

func NewDatabase(diskdb neatdb.KeyValueStore) *Database {
	return NewDatabaseWithCache(diskdb, 0)
}

func NewDatabaseWithCache(diskdb neatdb.KeyValueStore, cache int) *Database {
	var cleans *bigcache.BigCache
	if cache > 0 {
		cleans, _ = bigcache.NewBigCache(bigcache.Config{
			Shards:             1024,
			LifeWindow:         time.Hour,
			MaxEntriesInWindow: cache * 1024,
			MaxEntrySize:       512,
			HardMaxCacheSize:   cache,
			Hasher:             trienodeHasher{},
		})
	}
	return &Database{
		diskdb:    diskdb,
		cleans:    cleans,
		dirties:   map[common.Hash]*cachedNode{{}: {}},
		preimages: make(map[common.Hash][]byte),
	}
}

func (db *Database) DiskDB() neatdb.Reader {
	return db.diskdb
}

func (db *Database) InsertBlob(hash common.Hash, blob []byte) {
	db.lock.Lock()
	defer db.lock.Unlock()

	db.insert(hash, blob, rawNode(blob))
}

func (db *Database) insert(hash common.Hash, blob []byte, node node) {

	if _, ok := db.dirties[hash]; ok {
		return
	}

	entry := &cachedNode{
		node:      simplifyNode(node),
		size:      uint16(len(blob)),
		flushPrev: db.newest,
	}
	for _, side := range entry.sides() {
		if c := db.dirties[side]; c != nil {
			c.parents++
		}
	}
	db.dirties[hash] = entry

	if db.oldest == (common.Hash{}) {
		db.oldest, db.newest = hash, hash
	} else {
		db.dirties[db.newest].flushNext, db.newest = hash, hash
	}
	db.dirtiesSize += common.StorageSize(common.HashLength + entry.size)
}

func (db *Database) insertPreimage(hash common.Hash, preimage []byte) {
	if _, ok := db.preimages[hash]; ok {
		return
	}
	db.preimages[hash] = common.CopyBytes(preimage)
	db.preimagesSize += common.StorageSize(common.HashLength + len(preimage))
}

func (db *Database) node(hash common.Hash) node {

	if db.cleans != nil {
		if enc, err := db.cleans.Get(string(hash[:])); err == nil && enc != nil {
			memcacheCleanHitMeter.Mark(1)
			memcacheCleanReadMeter.Mark(int64(len(enc)))
			return mustDecodeNode(hash[:], enc)
		}
	}

	db.lock.RLock()
	dirty := db.dirties[hash]
	db.lock.RUnlock()

	if dirty != nil {
		return dirty.obj(hash)
	}

	enc, err := db.diskdb.Get(hash[:])
	if err != nil || enc == nil {
		return nil
	}
	if db.cleans != nil {
		db.cleans.Set(string(hash[:]), enc)
		memcacheCleanMissMeter.Mark(1)
		memcacheCleanWriteMeter.Mark(int64(len(enc)))
	}
	return mustDecodeNode(hash[:], enc)
}

func (db *Database) Node(hash common.Hash) ([]byte, error) {

	if hash == (common.Hash{}) {
		return nil, errors.New("not found")
	}

	if db.cleans != nil {
		if enc, err := db.cleans.Get(string(hash[:])); err == nil && enc != nil {
			memcacheCleanHitMeter.Mark(1)
			memcacheCleanReadMeter.Mark(int64(len(enc)))
			return enc, nil
		}
	}

	db.lock.RLock()
	dirty := db.dirties[hash]
	db.lock.RUnlock()

	if dirty != nil {
		return dirty.rlp(), nil
	}

	enc, err := db.diskdb.Get(hash[:])
	if err == nil && enc != nil {
		if db.cleans != nil {
			db.cleans.Set(string(hash[:]), enc)
			memcacheCleanMissMeter.Mark(1)
			memcacheCleanWriteMeter.Mark(int64(len(enc)))
		}
	}
	return enc, err
}

func (db *Database) preimage(hash common.Hash) ([]byte, error) {

	db.lock.RLock()
	preimage := db.preimages[hash]
	db.lock.RUnlock()

	if preimage != nil {
		return preimage, nil
	}

	return db.diskdb.Get(db.secureKey(hash[:]))
}

func (db *Database) secureKey(key []byte) []byte {

	buf := append(secureKeyPrefix[:], key...)
	return buf
}

func (db *Database) Nodes() []common.Hash {
	db.lock.RLock()
	defer db.lock.RUnlock()

	var hashes = make([]common.Hash, 0, len(db.dirties))
	for hash := range db.dirties {
		if hash != (common.Hash{}) {
			hashes = append(hashes, hash)
		}
	}
	return hashes
}

func (db *Database) Reference(side common.Hash, parent common.Hash) {
	db.lock.Lock()
	defer db.lock.Unlock()

	db.reference(side, parent)
}

func (db *Database) reference(side common.Hash, parent common.Hash) {

	node, ok := db.dirties[side]
	if !ok {
		return
	}

	if db.dirties[parent].sideren == nil {
		db.dirties[parent].sideren = make(map[common.Hash]uint16)
	} else if _, ok = db.dirties[parent].sideren[side]; ok && parent != (common.Hash{}) {
		return
	}
	node.parents++
	db.dirties[parent].sideren[side]++
}

func (db *Database) Dereference(root common.Hash) {

	if root == (common.Hash{}) {
		log.Error("Attempted to dereference the trie cache meta root")
		return
	}
	db.lock.Lock()
	defer db.lock.Unlock()

	nodes, storage, start := len(db.dirties), db.dirtiesSize, time.Now()
	db.dereference(root, common.Hash{})

	db.gcnodes += uint64(nodes - len(db.dirties))
	db.gcsize += storage - db.dirtiesSize
	db.gctime += time.Since(start)

	memcacheGCTimeTimer.Update(time.Since(start))
	memcacheGCSizeMeter.Mark(int64(storage - db.dirtiesSize))
	memcacheGCNodesMeter.Mark(int64(nodes - len(db.dirties)))

	log.Debug("Dereferenced trie from memory database", "nodes", nodes-len(db.dirties), "size", storage-db.dirtiesSize, "time", time.Since(start),
		"gcnodes", db.gcnodes, "gcsize", db.gcsize, "gctime", db.gctime, "livenodes", len(db.dirties), "livesize", db.dirtiesSize)
}

func (db *Database) dereference(side common.Hash, parent common.Hash) {

	node := db.dirties[parent]

	if node.sideren != nil && node.sideren[side] > 0 {
		node.sideren[side]--
		if node.sideren[side] == 0 {
			delete(node.sideren, side)
		}
	}

	node, ok := db.dirties[side]
	if !ok {
		return
	}

	if node.parents > 0 {

		node.parents--
	}
	if node.parents == 0 {

		switch side {
		case db.oldest:
			db.oldest = node.flushNext
			db.dirties[node.flushNext].flushPrev = common.Hash{}
		case db.newest:
			db.newest = node.flushPrev
			db.dirties[node.flushPrev].flushNext = common.Hash{}
		default:
			db.dirties[node.flushPrev].flushNext = node.flushNext
			db.dirties[node.flushNext].flushPrev = node.flushPrev
		}

		for _, hash := range node.sides() {
			db.dereference(hash, side)
		}
		delete(db.dirties, side)
		db.dirtiesSize -= common.StorageSize(common.HashLength + int(node.size))
	}
}

func (db *Database) Cap(limit common.StorageSize) error {

	nodes, storage, start := len(db.dirties), db.dirtiesSize, time.Now()
	batch := db.diskdb.NewBatch()

	size := db.dirtiesSize + common.StorageSize((len(db.dirties)-1)*2*common.HashLength)

	flushPreimages := db.preimagesSize > 4*1024*1024
	if flushPreimages {
		for hash, preimage := range db.preimages {
			if err := batch.Put(db.secureKey(hash[:]), preimage); err != nil {
				log.Error("Failed to commit preimage from trie database", "err", err)
				return err
			}
			if batch.ValueSize() > neatdb.IdealBatchSize {
				if err := batch.Write(); err != nil {
					return err
				}
				batch.Reset()
			}
		}
	}

	oldest := db.oldest
	for size > limit && oldest != (common.Hash{}) {

		node := db.dirties[oldest]
		if err := batch.Put(oldest[:], node.rlp()); err != nil {
			return err
		}

		if batch.ValueSize() >= neatdb.IdealBatchSize {
			if err := batch.Write(); err != nil {
				log.Error("Failed to write flush list to disk", "err", err)
				return err
			}
			batch.Reset()
		}

		size -= common.StorageSize(3*common.HashLength + int(node.size))
		oldest = node.flushNext
	}

	if err := batch.Write(); err != nil {
		log.Error("Failed to write flush list to disk", "err", err)
		return err
	}

	db.lock.Lock()
	defer db.lock.Unlock()

	if flushPreimages {
		db.preimages = make(map[common.Hash][]byte)
		db.preimagesSize = 0
	}
	for db.oldest != oldest {
		node := db.dirties[db.oldest]
		delete(db.dirties, db.oldest)
		db.oldest = node.flushNext

		db.dirtiesSize -= common.StorageSize(common.HashLength + int(node.size))
	}
	if db.oldest != (common.Hash{}) {
		db.dirties[db.oldest].flushPrev = common.Hash{}
	}
	db.flushnodes += uint64(nodes - len(db.dirties))
	db.flushsize += storage - db.dirtiesSize
	db.flushtime += time.Since(start)

	memcacheFlushTimeTimer.Update(time.Since(start))
	memcacheFlushSizeMeter.Mark(int64(storage - db.dirtiesSize))
	memcacheFlushNodesMeter.Mark(int64(nodes - len(db.dirties)))

	log.Debug("Persisted nodes from memory database", "nodes", nodes-len(db.dirties), "size", storage-db.dirtiesSize, "time", time.Since(start),
		"flushnodes", db.flushnodes, "flushsize", db.flushsize, "flushtime", db.flushtime, "livenodes", len(db.dirties), "livesize", db.dirtiesSize)

	return nil
}

func (db *Database) Commit(node common.Hash, report bool) error {

	start := time.Now()
	batch := db.diskdb.NewBatch()

	for hash, preimage := range db.preimages {
		if err := batch.Put(db.secureKey(hash[:]), preimage); err != nil {
			log.Error("Failed to commit preimage from trie database", "err", err)
			return err
		}

		if batch.ValueSize() > neatdb.IdealBatchSize {
			if err := batch.Write(); err != nil {
				return err
			}
			batch.Reset()
		}
	}

	if err := batch.Write(); err != nil {
		return err
	}
	batch.Reset()

	nodes, storage := len(db.dirties), db.dirtiesSize

	uncacher := &cleaner{db}
	if err := db.commit(node, batch, uncacher); err != nil {
		log.Error("Failed to commit trie from trie database", "err", err)
		return err
	}

	if err := batch.Write(); err != nil {
		log.Error("Failed to write trie to disk", "err", err)
		return err
	}

	db.lock.Lock()
	defer db.lock.Unlock()

	batch.Replay(uncacher)
	batch.Reset()

	db.preimages = make(map[common.Hash][]byte)
	db.preimagesSize = 0

	memcacheCommitTimeTimer.Update(time.Since(start))
	memcacheCommitSizeMeter.Mark(int64(storage - db.dirtiesSize))
	memcacheCommitNodesMeter.Mark(int64(nodes - len(db.dirties)))

	logger := log.Info
	if !report {
		logger = log.Debug
	}
	logger("Persisted trie from memory database", "nodes", nodes-len(db.dirties)+int(db.flushnodes), "size", storage-db.dirtiesSize+db.flushsize, "time", time.Since(start)+db.flushtime,
		"gcnodes", db.gcnodes, "gcsize", db.gcsize, "gctime", db.gctime, "livenodes", len(db.dirties), "livesize", db.dirtiesSize)

	db.gcnodes, db.gcsize, db.gctime = 0, 0, 0
	db.flushnodes, db.flushsize, db.flushtime = 0, 0, 0

	return nil
}

func (db *Database) commit(hash common.Hash, batch neatdb.Batch, uncacher *cleaner) error {

	node, ok := db.dirties[hash]
	if !ok {
		return nil
	}
	for _, side := range node.sides() {
		if err := db.commit(side, batch, uncacher); err != nil {
			return err
		}
	}
	if err := batch.Put(hash[:], node.rlp()); err != nil {
		return err
	}

	if batch.ValueSize() >= neatdb.IdealBatchSize {
		if err := batch.Write(); err != nil {
			return err
		}
		db.lock.Lock()
		batch.Replay(uncacher)
		batch.Reset()
		db.lock.Unlock()
	}
	return nil
}

type cleaner struct {
	db *Database
}

func (c *cleaner) Put(key []byte, rlp []byte) error {
	hash := common.BytesToHash(key)

	node, ok := c.db.dirties[hash]
	if !ok {
		return nil
	}

	switch hash {
	case c.db.oldest:
		c.db.oldest = node.flushNext
		c.db.dirties[node.flushNext].flushPrev = common.Hash{}
	case c.db.newest:
		c.db.newest = node.flushPrev
		c.db.dirties[node.flushPrev].flushNext = common.Hash{}
	default:
		c.db.dirties[node.flushPrev].flushNext = node.flushNext
		c.db.dirties[node.flushNext].flushPrev = node.flushPrev
	}

	delete(c.db.dirties, hash)
	c.db.dirtiesSize -= common.StorageSize(common.HashLength + int(node.size))

	if c.db.cleans != nil {
		c.db.cleans.Set(string(hash[:]), rlp)
	}
	return nil
}

func (c *cleaner) Delete(key []byte) error {
	panic("Not implemented")
}

func (db *Database) Size() (common.StorageSize, common.StorageSize) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	var flushlistSize = common.StorageSize((len(db.dirties) - 1) * 2 * common.HashLength)
	return db.dirtiesSize + flushlistSize, db.preimagesSize
}

func (db *Database) verifyIntegrity() {

	reachable := map[common.Hash]struct{}{{}: {}}

	for side := range db.dirties[common.Hash{}].sideren {
		db.accumulate(side, reachable)
	}

	var unreachable []string
	for hash, node := range db.dirties {
		if _, ok := reachable[hash]; !ok {
			unreachable = append(unreachable, fmt.Sprintf("%x: {Node: %v, Parents: %d, Prev: %x, Next: %x}",
				hash, node.node, node.parents, node.flushPrev, node.flushNext))
		}
	}
	if len(unreachable) != 0 {
		panic(fmt.Sprintf("trie cache memory leak: %v", unreachable))
	}
}

func (db *Database) accumulate(hash common.Hash, reachable map[common.Hash]struct{}) {

	node, ok := db.dirties[hash]
	if !ok {
		return
	}
	reachable[hash] = struct{}{}

	for _, side := range node.sides() {
		db.accumulate(side, reachable)
	}
}
