package core

import (
	"fmt"

	"github.com/neatlab/neatio/chain/consensus"
	tmTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/core/types"
)

// Consider moving the apply logic to each op (how to avoid import circular reference?)
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
	case *tmTypes.SwitchEpochOp:
		eng := bc.engine.(consensus.NeatCon)
		nextEp, err := eng.GetEpoch().EnterNewEpoch(op.NewValidators)
		if err == nil {
			// Stop the Engine if we are not in the new validators
			if !op.NewValidators.HasAddress(eng.PrivateValidator().Bytes()) && eng.IsStarted() {
				bc.PostChainEvents([]interface{}{StopMiningEvent{}}, nil)
			}

			//Start the Engine if we are in the new validators
			if op.NewValidators.HasAddress(eng.PrivateValidator().Bytes()) && !eng.IsStarted() {
				bc.PostChainEvents([]interface{}{StartMiningEvent{}}, nil)
			}

			eng.SetEpoch(nextEp)
			cch.ChangeValidators(op.ChainId) //must after eng.SetEpoch(nextEp), it uses epoch just set
		}
		return err
	default:
		return fmt.Errorf("unknown op: %v", op)
	}
}
