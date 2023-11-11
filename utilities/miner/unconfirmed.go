package miner

import (
	"container/ring"
	"sync"

	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/utilities/common"
)

type headerRetriever interface {
	GetHeaderByNumber(number uint64) *types.Header
}

type unconfirmedBlock struct {
	index uint64
	hash  common.Hash
}

type unconfirmedBlocks struct {
	chain  headerRetriever
	depth  uint
	blocks *ring.Ring
	lock   sync.RWMutex
	logger log.Logger
}

func newUnconfirmedBlocks(chain headerRetriever, depth uint, logger log.Logger) *unconfirmedBlocks {
	return &unconfirmedBlocks{
		chain:  chain,
		depth:  depth,
		logger: logger,
	}
}

func (set *unconfirmedBlocks) Insert(index uint64, hash common.Hash) {

	set.Shift(index)

	item := ring.New(1)
	item.Value = &unconfirmedBlock{
		index: index,
		hash:  hash,
	}

	set.lock.Lock()
	defer set.lock.Unlock()

	if set.blocks == nil {
		set.blocks = item
	} else {
		set.blocks.Move(-1).Link(item)
	}

}

func (set *unconfirmedBlocks) Shift(height uint64) {
	set.lock.Lock()
	defer set.lock.Unlock()

	for set.blocks != nil {

		next := set.blocks.Value.(*unconfirmedBlock)
		if next.index+uint64(set.depth) > height {
			break
		}

		header := set.chain.GetHeaderByNumber(next.index)
		switch {
		case header == nil:
			set.logger.Warn("Failed to retrieve header of mined block", "number", next.index, "hash", next.hash)
		case header.Hash() == next.hash:

		default:
			set.logger.Info("⑂ Block became a side fork", "number", next.index, "hash", next.hash)
		}

		if set.blocks.Value == set.blocks.Next().Value {
			set.blocks = nil
		} else {
			set.blocks = set.blocks.Move(-1)
			set.blocks.Unlink(1)
			set.blocks = set.blocks.Move(1)
		}
	}
}
