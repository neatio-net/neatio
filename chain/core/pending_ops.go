package core

import (
	"fmt"

	"github.com/neatlab/neatio/chain/consensus"
	ncTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/core/types"
)

func ApplyOp(op types.PendingOp, bc *BlockChain, cch CrossChainHelper) error {
	switch op := op.(type) {
	case *types.CreateSideChainOp:
		return cch.CreateSideChain(op.From, op.ChainId, op.MinValidators, op.MinDepositAmount, op.StartBlock, op.EndBlock)
	case *types.JoinSideChainOp:
		return cch.JoinSideChain(op.From, op.PubKey, op.ChainId, op.DepositAmount)
	case *types.LaunchSideChainsOp:
		if len(op.SideChainIds) > 0 {
			var events []interface{}
			for _, sideChainId := range op.SideChainIds {
				events = append(events, CreateSideChainEvent{ChainId: sideChainId})
			}
			bc.PostChainEvents(events, nil)
		}
		if op.NewPendingIdx != nil || len(op.DeleteSideChainIds) > 0 {
			cch.ProcessPostPendingData(op.NewPendingIdx, op.DeleteSideChainIds)
		}
		return nil
	case *types.VoteNextEpochOp:
		ep := bc.engine.(consensus.NeatCon).GetEpoch()
		ep = ep.GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
		return cch.VoteNextEpoch(ep, op.From, op.VoteHash, op.TxHash)
	case *types.RevealVoteOp:
		ep := bc.engine.(consensus.NeatCon).GetEpoch()
		ep = ep.GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
		return cch.RevealVote(ep, op.From, op.Pubkey, op.Amount, op.Salt, op.TxHash)
	case *types.UpdateNextEpochOp:
		ep := bc.engine.(consensus.NeatCon).GetEpoch()
		ep = ep.GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
		return cch.UpdateNextEpoch(ep, op.From, op.PubKey, op.Amount, op.Salt, op.TxHash)
	case *types.SaveDataToMainChainOp:
		return cch.SaveSideChainProofDataToMainChain(op.Data)
	case *ncTypes.SwitchEpochOp:
		eng := bc.engine.(consensus.NeatCon)
		nextEp, err := eng.GetEpoch().EnterNewEpoch(op.NewValidators)
		if err == nil {

			if !op.NewValidators.HasAddress(eng.PrivateValidator().Bytes()) && eng.IsStarted() {
				bc.PostChainEvents([]interface{}{StopMiningEvent{}}, nil)
			}

			if op.NewValidators.HasAddress(eng.PrivateValidator().Bytes()) && !eng.IsStarted() {
				bc.PostChainEvents([]interface{}{StartMiningEvent{}}, nil)
			}

			eng.SetEpoch(nextEp)
			cch.ChangeValidators(op.ChainId)
		}
		return err
	default:
		return fmt.Errorf("unknown op: %v", op)
	}
}
