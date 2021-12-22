package consensus

import (
	"math/big"

	"github.com/neatlab/neatio/chain/consensus/neatcon/epoch"
	"github.com/neatlab/neatio/chain/core/state"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/network/rpc"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
)

type ChainReader interface {
	Config() *params.ChainConfig

	CurrentHeader() *types.Header

	GetHeader(hash common.Hash, number uint64) *types.Header

	GetHeaderByNumber(number uint64) *types.Header

	GetHeaderByHash(hash common.Hash) *types.Header

	GetBlock(hash common.Hash, number uint64) *types.Block

	GetBlockByNumber(number uint64) *types.Block

	GetTd(hash common.Hash, number uint64) *big.Int

	CurrentBlock() *types.Block

	State() (*state.StateDB, error)
}

type ChainValidator interface {
	ValidateBlock(block *types.Block) (*state.StateDB, types.Receipts, *types.PendingOps, error)
}

type Engine interface {
	Author(header *types.Header) (common.Address, error)

	VerifyHeader(chain ChainReader, header *types.Header, seal bool) error

	VerifyHeaders(chain ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	VerifyUncles(chain ChainReader, block *types.Block) error

	VerifySeal(chain ChainReader, header *types.Header) error

	Prepare(chain ChainReader, header *types.Header) error

	Finalize(chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, totalGasFee *big.Int,
		uncles []*types.Header, receipts []*types.Receipt, ops *types.PendingOps) (*types.Block, error)

	Seal(chain ChainReader, block *types.Block, stop <-chan struct{}) (interface{}, error)

	CalcDifficulty(chain ChainReader, time uint64, parent *types.Header) *big.Int

	APIs(chain ChainReader) []rpc.API

	Close() error

	Protocol() Protocol
}

type Handler interface {
	NewChainHead(block *types.Block) error

	HandleMsg(chID uint64, src Peer, msgBytes []byte) (bool, error)

	SetBroadcaster(Broadcaster)

	GetBroadcaster() Broadcaster

	AddPeer(src Peer)

	RemovePeer(src Peer)
}

type EngineStartStop interface {
	Start(chain ChainReader, currentBlock func() *types.Block, hasBadBlock func(hash common.Hash) bool) error

	Stop() error
}

type NeatCon interface {
	Engine

	EngineStartStop

	ShouldStart() bool

	IsStarted() bool

	ForceStart()

	GetEpoch() *epoch.Epoch

	SetEpoch(ep *epoch.Epoch)

	PrivateValidator() common.Address

	VerifyHeaderBeforeConsensus(chain ChainReader, header *types.Header, seal bool) error
}
