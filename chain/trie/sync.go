// Copyright 2015 The go-ethereum Authors
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
	"errors"
	"fmt"

	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/prque"
)

var ErrNotRequested = errors.New("not requested")

var ErrAlreadyProcessed = errors.New("already processed")

type request struct {
	hash common.Hash
	data []byte
	raw  bool

	parents []*request
	depth   int
	deps    int

	callback LeafCallback
}

type SyncResult struct {
	Hash common.Hash
	Data []byte
}

type syncMemBatch struct {
	batch map[common.Hash][]byte
	order []common.Hash
}

func newSyncMemBatch() *syncMemBatch {
	return &syncMemBatch{
		batch: make(map[common.Hash][]byte),
		order: make([]common.Hash, 0, 256),
	}
}

type Sync struct {
	database neatdb.Reader
	membatch *syncMemBatch
	requests map[common.Hash]*request
	queue    *prque.Prque
}

func NewSync(root common.Hash, database neatdb.Reader, callback LeafCallback) *Sync {
	ts := &Sync{
		database: database,
		membatch: newSyncMemBatch(),
		requests: make(map[common.Hash]*request),
		queue:    prque.New(nil),
	}
	ts.AddSubTrie(root, 0, common.Hash{}, callback)
	return ts
}

func (s *Sync) AddSubTrie(root common.Hash, depth int, parent common.Hash, callback LeafCallback) {

	if root == emptyRoot {
		return
	}
	if _, ok := s.membatch.batch[root]; ok {
		return
	}
	key := root.Bytes()
	blob, _ := s.database.Get(key)
	if local, err := decodeNode(key, blob); local != nil && err == nil {
		return
	}

	req := &request{
		hash:     root,
		depth:    depth,
		callback: callback,
	}

	if parent != (common.Hash{}) {
		ancestor := s.requests[parent]
		if ancestor == nil {
			panic(fmt.Sprintf("sub-trie ancestor not found: %x", parent))
		}
		ancestor.deps++
		req.parents = append(req.parents, ancestor)
	}
	s.schedule(req)
}

func (s *Sync) AddRawEntry(hash common.Hash, depth int, parent common.Hash) {

	if hash == emptyState {
		return
	}
	if _, ok := s.membatch.batch[hash]; ok {
		return
	}
	if ok, _ := s.database.Has(hash.Bytes()); ok {
		return
	}

	req := &request{
		hash:  hash,
		raw:   true,
		depth: depth,
	}

	if parent != (common.Hash{}) {
		ancestor := s.requests[parent]
		if ancestor == nil {
			panic(fmt.Sprintf("raw-entry ancestor not found: %x", parent))
		}
		ancestor.deps++
		req.parents = append(req.parents, ancestor)
	}
	s.schedule(req)
}

func (s *Sync) Missing(max int) []common.Hash {
	var requests []common.Hash
	for !s.queue.Empty() && (max == 0 || len(requests) < max) {
		requests = append(requests, s.queue.PopItem().(common.Hash))
	}
	return requests
}

func (s *Sync) Process(results []SyncResult) (bool, int, error) {
	committed := false

	for i, item := range results {

		request := s.requests[item.Hash]
		if request == nil {
			return committed, i, ErrNotRequested
		}
		if request.data != nil {
			return committed, i, ErrAlreadyProcessed
		}

		if request.raw {
			request.data = item.Data
			s.commit(request)
			committed = true
			continue
		}

		node, err := decodeNode(item.Hash[:], item.Data)
		if err != nil {
			return committed, i, err
		}
		request.data = item.Data

		requests, err := s.sideren(request, node)
		if err != nil {
			return committed, i, err
		}
		if len(requests) == 0 && request.deps == 0 {
			s.commit(request)
			committed = true
			continue
		}
		request.deps += len(requests)
		for _, side := range requests {
			s.schedule(side)
		}
	}
	return committed, 0, nil
}

func (s *Sync) Commit(dbw neatdb.Writer) (int, error) {

	for i, key := range s.membatch.order {
		if err := dbw.Put(key[:], s.membatch.batch[key]); err != nil {
			return i, err
		}
	}
	written := len(s.membatch.order)

	s.membatch = newSyncMemBatch()
	return written, nil
}

func (s *Sync) Pending() int {
	return len(s.requests)
}

func (s *Sync) schedule(req *request) {

	if old, ok := s.requests[req.hash]; ok {
		old.parents = append(old.parents, req.parents...)
		return
	}

	s.queue.Push(req.hash, int64(req.depth))
	s.requests[req.hash] = req
}

func (s *Sync) sideren(req *request, object node) ([]*request, error) {

	type side struct {
		node  node
		depth int
	}
	var sideren []side

	switch node := (object).(type) {
	case *shortNode:
		sideren = []side{{
			node:  node.Val,
			depth: req.depth + len(node.Key),
		}}
	case *fullNode:
		for i := 0; i < 17; i++ {
			if node.Children[i] != nil {
				sideren = append(sideren, side{
					node:  node.Children[i],
					depth: req.depth + 1,
				})
			}
		}
	default:
		panic(fmt.Sprintf("unknown node: %+v", node))
	}

	requests := make([]*request, 0, len(sideren))
	for _, side := range sideren {

		if req.callback != nil {
			if node, ok := (side.node).(valueNode); ok {
				if err := req.callback(node, req.hash); err != nil {
					return nil, err
				}
			}
		}

		if node, ok := (side.node).(hashNode); ok {

			hash := common.BytesToHash(node)
			if _, ok := s.membatch.batch[hash]; ok {
				continue
			}
			if ok, _ := s.database.Has(node); ok {
				continue
			}

			requests = append(requests, &request{
				hash:     hash,
				parents:  []*request{req},
				depth:    side.depth,
				callback: req.callback,
			})
		}
	}
	return requests, nil
}

func (s *Sync) commit(req *request) (err error) {

	s.membatch.batch[req.hash] = req.data
	s.membatch.order = append(s.membatch.order, req.hash)

	delete(s.requests, req.hash)

	for _, parent := range req.parents {
		parent.deps--
		if parent.deps == 0 {
			if err := s.commit(parent); err != nil {
				return err
			}
		}
	}
	return nil
}
