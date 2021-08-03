package consensus

import (
	"time"

	cmn "github.com/Gessiux/go-common"
	consss "github.com/neatlab/neatio/chain/consensus"
	ep "github.com/neatlab/neatio/chain/consensus/neatcon/epoch"
	sm "github.com/neatlab/neatio/chain/consensus/neatcon/state"
	"github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/params"
)

// The +2/3 and other Precommit-votes for block at `height`.
// This Commit comes from block.LastCommit for `height+1`.
func (bs *ConsensusState) GetChainReader() consss.ChainReader {
	return bs.backend.ChainReader()
}

//this function is called when the system starts or a block has been inserted into
//the insert could be self/other triggered
//anyway, we start/restart a new height with the latest block update
func (cs *ConsensusState) StartNewHeight() {

	//start locking
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	//reload the block
	cr := cs.backend.ChainReader()
	curEthBlock := cr.CurrentBlock()
	curHeight := curEthBlock.NumberU64()
	cs.logger.Infof("StartNewHeight. current block height is %v", curHeight)

	state := cs.InitState(cs.Epoch)
	cs.UpdateToState(state)

	cs.newStep()
	cs.scheduleRound0(cs.getRoundState()) //not use cs.GetRoundState to avoid dead-lock
}

func (cs *ConsensusState) InitState(epoch *ep.Epoch) *sm.State {

	state := sm.NewState(cs.logger)

	state.NTCExtra, _ = cs.LoadLastNeatConExtra()
	if state.NTCExtra == nil { //means it it the first block

		state = sm.MakeGenesisState( /*stateDB, */ cs.chainConfig.NeatChainId, cs.logger)
		//state.Save()

		if state.NTCExtra.EpochNumber != uint64(epoch.Number) {
			cmn.Exit(cmn.Fmt("InitStateAndEpoch(), initial state error"))
		}
		state.Epoch = epoch

		cs.logger.Infof("InitStateAndEpoch. genesis state extra: %#v, epoch validators: %v", state.NTCExtra, epoch.Validators)
	} else {
		state.Epoch = epoch
		cs.ReconstructLastCommit(state)

		cs.logger.Infof("InitStateAndEpoch. state extra: %#v, epoch validators: %v", state.NTCExtra, epoch.Validators)
	}

	return state
}

func (cs *ConsensusState) Initialize() {

	//initialize state
	cs.Height = 0

	//initialize round state
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

// Updates ConsensusState and increments height to match thatRewardScheme of state.
// The round becomes 0 and cs.Step becomes RoundStepNewHeight.
func (cs *ConsensusState) UpdateToState(state *sm.State) {

	cs.Initialize()

	height := state.NTCExtra.Height + 1
	// Next desired block height
	cs.Height = height

	if cs.blockFromMiner != nil && cs.blockFromMiner.NumberU64() >= cs.Height {
		log.Debugf("block %v has been received from miner, not set to nil", cs.blockFromMiner.NumberU64())
	} else {
		cs.blockFromMiner = nil
	}

	// RoundState fields
	cs.updateRoundStep(0, RoundStepNewHeight)
	//cs.StartTime = cs.timeoutParams.Commit(cs.CommitTime)

	if state.NTCExtra.ChainID == params.MainnetChainConfig.NeatChainId ||
		state.NTCExtra.ChainID == params.TestnetChainConfig.NeatChainId {
		cs.StartTime = cs.timeoutParams.Commit(time.Now())
	} else {
		if cs.CommitTime.IsZero() {
			// "Now" makes it easier to sync up dev nodes.
			// We add timeoutCommit to allow transactions
			// to be gathered for the first block.
			// And alternative solution that relies on clocks:
			//  cs.StartTime = state.LastBlockTime.Add(timeoutCommit)
			cs.StartTime = cs.timeoutParams.Commit(time.Now())
		} else {
			cs.StartTime = cs.timeoutParams.Commit(cs.CommitTime)
		}
	}

	// Reset fields based on state.
	_, validators, _ := state.GetValidators()
	cs.Validators = validators
	cs.Votes = NewHeightVoteSet(cs.chainConfig.NeatChainId, height, validators, cs.logger)
	cs.VoteSignAggr = NewHeightVoteSignAggr(cs.chainConfig.NeatChainId, height, validators, cs.logger)

	cs.vrfValIndex = -1
	cs.pastRoundStates = make(map[int]int)

	cs.state = state

	cs.newStep()
}

// The +2/3 and other Precommit-votes for block at `height`.
// This Commit comes from block.LastCommit for `height+1`.
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
