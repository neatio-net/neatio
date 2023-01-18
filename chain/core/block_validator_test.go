package core

import (
	"fmt"

	"github.com/neatio-network/neatio/chain/consensus"
	"github.com/neatio-network/neatio/chain/core/state"
	"github.com/neatio-network/neatio/chain/core/types"
	"github.com/neatio-network/neatio/params"
)

type BlockValidator struct {
	config *params.ChainConfig
	bc     *BlockChain
	engine consensus.Engine
	core   *NeatCon
}


func (v *BlockValidator) ValidateBody(block *types.Block) error {

	if v.bc.HasBlockAndState(block.Hash(), block.NumberU64()) {

		return ErrKnownBlock

	}

	header := block.Header()
	if err := v.engine.VerifyUncles(v.bc, block); err != nil {
		return err
	}
	if hash := types.CalcUncleHash(block.Uncles()); hash != header.UncleHash {
		return fmt.Errorf("uncle root hash mismatch: have %x, want %x", hash, header.UncleHash)
	}
	if hash := types.DeriveSha(block.Transactions()); hash != header.TxHash {
		return fmt.Errorf("transaction root hash mismatch: have %x, want %x", hash, header.TxHash)
	}
	if !v.bc.HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
		if !v.bc.HasBlock(block.ParentHash(), block.NumberU64()-1) {
			return consensus.ErrUnknownAncestor
		}
		return consensus.ErrPrunedAncestor
	}
	return nil
}


func CalcGasLimit(parent *types.Block, gasFloor, gasCeil uint64) uint64 {

	contrib := (parent.GasUsed() + parent.GasUsed()/2) / params.GasLimitBoundDivisor

	decay := parent.GasLimit()/params.GasLimitBoundDivisor - 1

	limit := parent.GasLimit() - decay + contrib
	if limit < params.MinGasLimit {
		limit = params.MinGasLimit
	}

	if limit < gasFloor {
		limit = parent.GasLimit() + decay
		if limit > gasFloor {
			limit = gasFloor
		}
	} else if limit > gasCeil {
		limit = parent.GasLimit() - decay
		if limit < gasCeil {
			limit = gasCeil
		}
	}
	return limit

	fmt.Printf("byte gas=%v\n", gas)
}
