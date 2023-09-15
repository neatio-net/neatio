package consensus

import (
	"time"

	cmn "github.com/nio-net/common"
	consss "github.com/nio-net/nio/chain/consensus"
	ep "github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	sm "github.com/nio-net/nio/chain/consensus/neatcon/state"
	"github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/params"
)

func (bs *ConsensusState) GetChainReader() consss.ChainReader {
	return bs.backend.ChainReader()
}

func (cs *ConsensusState) StartNewHeight() {

	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	state := cs.InitState(cs.Epoch)
	cs.UpdateToState(state)

	cs.newStep()
	cs.scheduleRound0(cs.getRoundState())
}

func (cs *ConsensusState) InitState(epoch *ep.Epoch) *sm.State {

	state := sm.NewState(cs.logger)

	state.NTCExtra, _ = cs.LoadLastNeatConExtra()
	if state.NTCExtra == nil {

		state = sm.MakeGenesisState(cs.chainConfig.NeatChainId, cs.logger)

		if state.NTCExtra.EpochNumber != uint64(epoch.Number) {
			cmn.Exit(cmn.Fmt("InitStateAndEpoch(), initial state error"))
		}
		state.Epoch = epoch

	} else {
		state.Epoch = epoch
		cs.ReconstructLastCommit(state)

	}

	return state
}

func (cs *ConsensusState) Initialize() {

	cs.Height = 0

	cs.proposer = nil
	cs.isProposer = false
	cs.ProposerPeerKey = ""
	cs.Validators = nil
	cs.Proposal = nil
	cs.ProposalBlock = nil
	cs.ProposalBlockParts = nil
	cs.LockedRound = -1
	cs.LockedBlock = nil
	cs.LockedBlockParts = nil
	cs.Votes = nil
	cs.VoteSignAggr = nil
	cs.PrevoteMaj23SignAggr = nil
	cs.PrecommitMaj23SignAggr = nil
	cs.CommitRound = -1
	cs.state = nil
}

func (cs *ConsensusState) UpdateToState(state *sm.State) {

	cs.Initialize()

	height := state.NTCExtra.Height + 1

	cs.Height = height

	if cs.blockFromMiner != nil && cs.blockFromMiner.NumberU64() >= cs.Height {
		log.Debugf("block %v has been received from miner, not set to nil", cs.blockFromMiner.NumberU64())
	} else {
		cs.blockFromMiner = nil
	}

	cs.updateRoundStep(0, RoundStepNewHeight)

	if state.NTCExtra.ChainID == params.MainnetChainConfig.NeatChainId ||
		state.NTCExtra.ChainID == params.TestnetChainConfig.NeatChainId {
		cs.StartTime = cs.timeoutParams.Commit(time.Now())
	} else {
		if cs.CommitTime.IsZero() {

			cs.StartTime = cs.timeoutParams.Commit(time.Now())
		} else {
			cs.StartTime = cs.timeoutParams.Commit(cs.CommitTime)
		}
	}

	_, validators, _ := state.GetValidators()
	cs.Validators = validators
	cs.Votes = NewHeightVoteSet(cs.chainConfig.NeatChainId, height, validators, cs.logger)
	cs.VoteSignAggr = NewHeightVoteSignAggr(cs.chainConfig.NeatChainId, height, validators, cs.logger)

	cs.vrfValIndex = -1
	cs.pastRoundStates = make(map[int]int)

	cs.state = state

	cs.newStep()
}

func (cs *ConsensusState) LoadBlock(height uint64) *types.NCBlock {

	cr := cs.GetChainReader()

	ethBlock := cr.GetBlockByNumber(height)
	if ethBlock == nil {
		return nil
	}

	header := cr.GetHeader(ethBlock.Hash(), ethBlock.NumberU64())
	if header == nil {
		return nil
	}
	NTCExtra, err := types.ExtractNeatConExtra(header)
	if err != nil {
		return nil
	}

	return &types.NCBlock{
		Block:    ethBlock,
		NTCExtra: NTCExtra,
	}
}

func (cs *ConsensusState) LoadLastNeatConExtra() (*types.NeatConExtra, uint64) {

	cr := cs.backend.ChainReader()

	curEthBlock := cr.CurrentBlock()
	curHeight := curEthBlock.NumberU64()
	if curHeight == 0 {
		return nil, 0
	}

	return cs.LoadNeatConExtra(curHeight)
}

func (cs *ConsensusState) LoadNeatConExtra(height uint64) (*types.NeatConExtra, uint64) {

	cr := cs.backend.ChainReader()

	ethBlock := cr.GetBlockByNumber(height)
	if ethBlock == nil {
		cs.logger.Warn("LoadNeatConExtra. nil block")
		return nil, 0
	}

	header := cr.GetHeader(ethBlock.Hash(), ethBlock.NumberU64())
	ncExtra, err := types.ExtractNeatConExtra(header)
	if err != nil {
		cs.logger.Warnf("LoadNeatConExtra. error: %v", err)
		return nil, 0
	}

	blockHeight := ethBlock.NumberU64()
	if ncExtra.Height != blockHeight {
		cs.logger.Warnf("extra.height:%v, block.Number %v, reset it", ncExtra.Height, blockHeight)
		ncExtra.Height = blockHeight
	}

	return ncExtra, ncExtra.Height
}
