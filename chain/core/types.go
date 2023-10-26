package core

import (
	"github.com/nio-net/nio/chain/core/state"
	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/core/vm"
)

type Validator interface {
	ValidateBody(block *types.Block) error

	ValidateState(block *types.Block, state *state.StateDB, receipts types.Receipts, usedGas uint64) error
}

type Processor interface {
	Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, *types.PendingOps, error)
}
