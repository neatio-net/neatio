package state

import (
	"errors"
	"fmt"

	"github.com/nio-net/nio/chain/consensus"
	ep "github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	"github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/core"
	neatTypes "github.com/nio-net/nio/chain/core/types"
)

func (s *State) ValidateBlock(block *types.NCBlock) error {
	return s.validateBlock(block)
}

func (s *State) validateBlock(block *types.NCBlock) error {
	err := block.ValidateBasic(s.NTCExtra)
	if err != nil {
		return err
	}

	epoch := s.Epoch.GetEpochByBlockNumber(block.NTCExtra.Height)
	if epoch == nil || epoch.Validators == nil {
		return errors.New("no epoch for current block height")
	}

	valSet := epoch.Validators
	err = valSet.VerifyCommit(block.NTCExtra.ChainID, block.NTCExtra.Height,
		block.NTCExtra.SeenCommit)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	core.RegisterInsertBlockCb("UpdateLocalEpoch", updateLocalEpoch)
	core.RegisterInsertBlockCb("AutoStartMining", autoStartMining)
}

func updateLocalEpoch(bc *core.BlockChain, block *neatTypes.Block) {
	if block.NumberU64() == 0 {
		return
	}

	ncExtra, _ := types.ExtractNeatConExtra(block.Header())
	epochInBlock := ep.FromBytes(ncExtra.EpochBytes)

	eng := bc.Engine().(consensus.NeatCon)
	currentEpoch := eng.GetEpoch()

	if epochInBlock != nil {
		if epochInBlock.Number == currentEpoch.Number+1 {
			if block.NumberU64() == currentEpoch.StartBlock+1 || block.NumberU64() == 2 {
				epochInBlock.Status = ep.EPOCH_VOTED_NOT_SAVED
				epochInBlock.SetRewardScheme(currentEpoch.GetRewardScheme())
				currentEpoch.SetNextEpoch(epochInBlock)
			} else if block.NumberU64() == currentEpoch.EndBlock {
				nextEp := currentEpoch.GetNextEpoch()
				nextEp.Validators = epochInBlock.Validators
				nextEp.Status = ep.EPOCH_VOTED_NOT_SAVED
			}
			currentEpoch.Save()
		} else if epochInBlock.Number == currentEpoch.Number {

			currentEpoch.StartTime = epochInBlock.StartTime
			currentEpoch.Save()

			if currentEpoch.Number > 0 {
				currentEpoch.GetPreviousEpoch().EndTime = epochInBlock.StartTime
				ep.UpdateEpochEndTime(currentEpoch.GetDB(), currentEpoch.Number-1, epochInBlock.StartTime)
			}
		}
	}
}

func autoStartMining(bc *core.BlockChain, block *neatTypes.Block) {
	eng := bc.Engine().(consensus.NeatCon)
	currentEpoch := eng.GetEpoch()

	if block.NumberU64() == currentEpoch.EndBlock-1 {
		fmt.Printf("auto start mining first %v\n", block.Number())

		nextEp := currentEpoch.GetNextEpoch()
		state, _ := bc.State()
		nextValidators := currentEpoch.Validators.Copy()
		dryrunErr := ep.DryRunUpdateEpochValidatorSet(state, nextValidators, nextEp.GetEpochValidatorVoteSet())
		if dryrunErr != nil {
			panic("can not update the validator set base on the vote, error: " + dryrunErr.Error())
		}
		nextEp.Validators = nextValidators

		if nextValidators.HasAddress(eng.PrivateValidator().Bytes()) && !eng.IsStarted() {
			fmt.Printf("auto start mining first, post start mining event")
			bc.PostChainEvents([]interface{}{core.StartMiningEvent{}}, nil)
		}
	}
}
