package consensus

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/utilities/common"

	"context"

	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"

	. "github.com/nio-net/common"
	cfg "github.com/nio-net/config"
	tmdcrypto "github.com/nio-net/crypto"
	consss "github.com/nio-net/nio/chain/consensus"
	ep "github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	sm "github.com/nio-net/nio/chain/consensus/neatcon/state"
	"github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/core"
	neatTypes "github.com/nio-net/nio/chain/core/types"
	neatAbi "github.com/nio-net/nio/neatabi/abi"
	"github.com/nio-net/nio/params"
	"github.com/nio-net/nio/utilities/crypto"
	"github.com/nio-net/nio/utilities/rlp"
)

const ROUND_NOT_PROPOSED int = 0
const ROUND_PROPOSED int = 1

type Backend interface {
	Commit(proposal *types.NCBlock, seals [][]byte, isProposer func() bool) error
	ChainReader() consss.ChainReader
	GetBroadcaster() consss.Broadcaster
	GetLogger() log.Logger
}

type TimeoutParams struct {
	WaitForMinerBlock0 int
	Propose0           int
	ProposeDelta       int
	Prevote0           int
	PrevoteDelta       int
	Precommit0         int
	PrecommitDelta     int
	Commit0            int
	SkipTimeoutCommit  bool
}

func (tp *TimeoutParams) WaitForMinerBlock() time.Duration {
	return time.Duration(tp.WaitForMinerBlock0) * time.Millisecond
}

func (tp *TimeoutParams) Propose(round int) time.Duration {
	if round >= 5 {
		round = 4
	}
	return time.Duration(tp.Propose0+tp.ProposeDelta*round) * time.Millisecond
}

func (tp *TimeoutParams) Prevote(round int) time.Duration {
	if round < 5 {
		return time.Duration(tp.Prevote0+tp.PrevoteDelta*round) * time.Millisecond
	} else {
		return time.Duration(tp.Prevote0+tp.PrevoteDelta*int(math.Pow(1.5, float64(round-1)))) * time.Millisecond
	}
}

func (tp *TimeoutParams) Precommit(round int) time.Duration {
	if round < 5 {
		return time.Duration(tp.Precommit0+tp.PrecommitDelta*round) * time.Millisecond
	} else {
		return time.Duration(tp.Precommit0+tp.PrecommitDelta*int(math.Pow(1.5, float64(round-1)))) * time.Millisecond
	}
}

func (tp *TimeoutParams) Commit(t time.Time) time.Time {
	return t.Add(time.Duration(tp.Commit0) * time.Millisecond)
}

func InitTimeoutParamsFromConfig(config cfg.Config) *TimeoutParams {
	return &TimeoutParams{
		WaitForMinerBlock0: config.GetInt("timeout_wait_for_miner_block"),
		Propose0:           config.GetInt("timeout_propose"),
		ProposeDelta:       config.GetInt("timeout_propose_delta"),
		Prevote0:           config.GetInt("timeout_prevote"),
		PrevoteDelta:       config.GetInt("timeout_prevote_delta"),
		Precommit0:         config.GetInt("timeout_precommit"),
		PrecommitDelta:     config.GetInt("timeout_precommit_delta"),
		Commit0:            config.GetInt("timeout_commit"),
		SkipTimeoutCommit:  config.GetBool("skip_timeout_commit"),
	}
}

type VRFProposer struct {
	Height uint64
	Round  int

	valIndex int
	Proposer *types.Validator
}

func (propser *VRFProposer) Validate(height uint64, round int) bool {
	if propser.Height == height && propser.Round == round {
		return true
	} else {
		return false
	}
}

var (
	ErrMinerBlock               = errors.New("Miner block is nil")
	ErrInvalidProposalSignature = errors.New("Error invalid proposal signature")
	ErrInvalidProposalPOLRound  = errors.New("Error invalid proposal POL round")
	ErrAddingVote               = errors.New("Error adding vote")
	ErrVoteHeightMismatch       = errors.New("Error vote height mismatch")
	ErrInvalidSignatureAggr     = errors.New("Invalid signature aggregation")
	ErrDuplicateSignatureAggr   = errors.New("Duplicate signature aggregation")
	ErrNotMaj23SignatureAggr    = errors.New("Signature aggregation has no +2/3 power")
)

type RoundStepType uint8

const (
	RoundStepNewHeight         = RoundStepType(0x01)
	RoundStepNewRound          = RoundStepType(0x02)
	RoundStepWaitForMinerBlock = RoundStepType(0x03)
	RoundStepPropose           = RoundStepType(0x04)
	RoundStepPrevote           = RoundStepType(0x05)
	RoundStepPrevoteWait       = RoundStepType(0x06)
	RoundStepPrecommit         = RoundStepType(0x07)
	RoundStepPrecommitWait     = RoundStepType(0x08)
	RoundStepCommit            = RoundStepType(0x09)
)

func (rs RoundStepType) String() string {
	switch rs {
	case RoundStepNewHeight:
		return "RoundStepNewHeight"
	case RoundStepNewRound:
		return "RoundStepNewRound"
	case RoundStepPropose:
		return "RoundStepPropose"
	case RoundStepWaitForMinerBlock:
		return "RoundStepWaitForMinerBlock"
	case RoundStepPrevote:
		return "RoundStepPrevote"
	case RoundStepPrevoteWait:
		return "RoundStepPrevoteWait"
	case RoundStepPrecommit:
		return "RoundStepPrecommit"
	case RoundStepPrecommitWait:
		return "RoundStepPrecommitWait"
	case RoundStepCommit:
		return "RoundStepCommit"
	default:
		return "RoundStepUnknown"
	}
}

type RoundState struct {
	Height             uint64
	Round              int
	Step               RoundStepType
	StartTime          time.Time
	CommitTime         time.Time
	Validators         *types.ValidatorSet
	Proposal           *types.Proposal
	ProposalBlock      *types.NCBlock
	ProposalBlockParts *types.PartSet
	ProposerPeerKey    string
	LockedRound        int
	LockedBlock        *types.NCBlock
	LockedBlockParts   *types.PartSet
	Votes              *HeightVoteSet
	VoteSignAggr       *HeightVoteSignAggr
	CommitRound        int

	PrevoteMaj23SignAggr   *types.SignAggr
	PrecommitMaj23SignAggr *types.SignAggr

	proposer   *VRFProposer
	isProposer bool
}

func (rs *RoundState) RoundStateEvent() types.EventDataRoundState {
	edrs := types.EventDataRoundState{
		Height:     rs.Height,
		Round:      rs.Round,
		Step:       rs.Step.String(),
		RoundState: rs,
	}
	return edrs
}

func (rs *RoundState) String() string {
	return rs.StringIndented("")
}

func (rs *RoundState) StringIndented(indent string) string {
	return fmt.Sprintf(`RoundState{
%s  H:%v R:%v S:%v
%s  StartTime:     %v
%s  CommitTime:    %v
%s  Validators:    %v
%s  Proposal:      %v
%s  ProposalBlock: %v %v
%s  LockedRound:   %v
%s  LockedBlock:   %v %v
%s  Votes:         %v
%s}`,
		indent, rs.Height, rs.Round, rs.Step,
		indent, rs.StartTime,
		indent, rs.CommitTime,
		indent, rs.Validators.StringIndented(indent+"    "),
		indent, rs.Proposal,
		indent, rs.ProposalBlockParts.StringShort(), rs.ProposalBlock.StringShort(),
		indent, rs.LockedRound,
		indent, rs.LockedBlockParts.StringShort(), rs.LockedBlock.StringShort(),
		indent, rs.Votes.StringIndented(indent+"    "),
		indent)
}

func (rs *RoundState) StringShort() string {
	return fmt.Sprintf(`RoundState{H:%v R:%v S:%v ST:%v}`,
		rs.Height, rs.Round, rs.Step, rs.StartTime)
}

var (
	msgQueueSize = 1000
)

type msgInfo struct {
	Msg     ConsensusMessage `json:"msg"`
	PeerKey string           `json:"peer_key"`
}

type timeoutInfo struct {
	Duration time.Duration `json:"duration"`
	Height   uint64        `json:"height"`
	Round    int           `json:"round"`
	Step     RoundStepType `json:"step"`
}

func (ti *timeoutInfo) String() string {
	return fmt.Sprintf("%v ; %d/%d %v", ti.Duration, ti.Height, ti.Round, ti.Step)
}

type PrivValidator interface {
	GetAddress() []byte
	GetPubKey() tmdcrypto.PubKey
	SignVote(chainID string, vote *types.Vote) error
	SignProposal(chainID string, proposal *types.Proposal) error
}

type ConsensusState struct {
	BaseService

	chainConfig   *params.ChainConfig
	privValidator PrivValidator
	cch           core.CrossChainHelper

	mtx sync.Mutex
	RoundState
	Epoch           *ep.Epoch
	state           *sm.State
	vrfValIndex     int
	pastRoundStates map[int]int

	peerMsgQueue     chan msgInfo
	internalMsgQueue chan msgInfo
	timeoutTicker    TimeoutTicker
	timeoutParams    *TimeoutParams

	evsw types.EventSwitch

	nSteps int

	decideProposal func(height uint64, round int)
	doPrevote      func(height uint64, round int)
	setProposal    func(proposal *types.Proposal) error

	done chan struct{}

	blockFromMiner *neatTypes.Block
	backend        Backend

	conR *ConsensusReactor

	logger log.Logger
}

func NewConsensusState(backend Backend, config cfg.Config, chainConfig *params.ChainConfig, cch core.CrossChainHelper, epoch *ep.Epoch) *ConsensusState {
	cs := &ConsensusState{
		chainConfig:      chainConfig,
		cch:              cch,
		Epoch:            epoch,
		peerMsgQueue:     make(chan msgInfo, msgQueueSize),
		internalMsgQueue: make(chan msgInfo, msgQueueSize),
		timeoutTicker:    NewTimeoutTicker(backend.GetLogger()),
		timeoutParams:    InitTimeoutParamsFromConfig(config),
		blockFromMiner:   nil,
		backend:          backend,
		logger:           backend.GetLogger(),
	}

	cs.decideProposal = cs.defaultDecideProposal
	cs.doPrevote = cs.defaultDoPrevote
	cs.setProposal = cs.defaultSetProposal

	cs.BaseService = *NewBaseService(backend.GetLogger(), "ConsensusState", cs)
	return cs
}

func (cs *ConsensusState) SetEventSwitch(evsw types.EventSwitch) {
	cs.evsw = evsw
}

func (cs *ConsensusState) String() string {
	return Fmt("ConsensusState")
}

func (cs *ConsensusState) GetState() *sm.State {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	return cs.state.Copy()
}

func (cs *ConsensusState) GetRoundState() *RoundState {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	return cs.getRoundState()
}

func (cs *ConsensusState) getRoundState() *RoundState {
	rs := cs.RoundState
	return &rs
}

func (cs *ConsensusState) GetValidators() (uint64, []*types.Validator) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	_, val, _ := cs.state.GetValidators()
	return cs.state.NTCExtra.Height, val.Copy().Validators
}

func (cs *ConsensusState) SetPrivValidator(priv PrivValidator) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.privValidator = priv
}

func BytesToBig(data []byte) *big.Int {
	n := new(big.Int)
	n.SetBytes(data)
	return n
}

func (cs *ConsensusState) updateProposer() {

	if cs.proposer != nil && cs.Round != cs.proposer.Round {
		log.Debug("update proposer for changing round",
			"cs.proposer.Round", cs.proposer.Round, "cs.Round", cs.Round)
	}

	cs.proposer = cs.proposerByRound(cs.Round)
	cs.isProposer = bytes.Equal(cs.proposer.Proposer.Address, cs.privValidator.GetAddress())

	cs.logger.Debugf("proposer, privalidator are (%v, %v)\n", cs.proposer.Proposer, cs.privValidator)
	if cs.isProposer {
		cs.logger.Debugf("IsProposer() return true\n")
	} else {
		cs.logger.Debugf("IsProposer() return false\n")
	}

	log.Debug("update proposer", "height", cs.Height, "round", cs.Round, "idx", cs.proposer.valIndex)
}

func (cs *ConsensusState) proposerByRound(round int) *VRFProposer {

	byVRF := false
	if round == 0 {
		byVRF = true
	}

	proposer := &VRFProposer{}

	proposer.Height = cs.Height
	proposer.Round = round

	idx := -1
	if byVRF {

		if cs.vrfValIndex >= 0 {

			idx = cs.vrfValIndex
		} else {

			lastProposer, curProposer := cs.proposersByVRF()

			idx = curProposer

			if lastProposer >= 0 &&
				curProposer == lastProposer &&
				cs.state.NTCExtra != nil &&
				cs.state.NTCExtra.SeenCommit != nil &&
				cs.state.NTCExtra.SeenCommit.BitArray != nil &&
				!cs.state.NTCExtra.SeenCommit.BitArray.GetIndex(uint64(curProposer)) {
				idx = (idx + 1) % cs.Validators.Size()
			}

			cs.vrfValIndex = idx
		}

	} else {

		if cs.vrfValIndex < 0 {
			PanicConsensus(Fmt("cs.vrfValIndex should not be -1", "cs.vrfValIndex", cs.vrfValIndex))
		}
		idx = (cs.vrfValIndex + proposer.Round) % cs.Validators.Size()
	}

	if idx >= cs.Validators.Size() || idx < 0 {
		proposer.Proposer = nil
		PanicConsensus(Fmt("The index of proposer out of range", "index:", idx, "range:", cs.Validators.Size()))
	} else {
		proposer.valIndex = idx
		proposer.Proposer = cs.Validators.Validators[idx]
	}
	log.Debug("get proposer by round", "height", cs.Height, "round", round, "idx", idx)

	return proposer
}

func (cs *ConsensusState) proposersByVRF() (lastProposer int, curProposer int) {

	chainReader := cs.backend.ChainReader()
	header := chainReader.CurrentHeader()
	headerHash := header.Hash()

	curProposer = cs.proposerByVRF(headerHash, cs.Validators.Validators)

	headerHeight := header.Number.Uint64()
	if headerHeight == cs.Epoch.StartBlock {
		return -1, curProposer
	}

	if headerHeight > 0 {
		lastHeader := chainReader.GetHeaderByNumber(headerHeight - 1)
		lastHeaderHash := lastHeader.Hash()
		lastProposer = cs.proposerByVRF(lastHeaderHash, cs.Validators.Validators)
		return lastProposer, curProposer
	}

	return -1, -1
}

func (cs *ConsensusState) proposerByVRF(headerHash common.Hash, validators []*types.Validator) (proposer int) {

	idx := -1

	var roundBytes = make([]byte, 8)
	vrfBytes := append(roundBytes, headerHash[:]...)
	hs := sha256.New()
	hs.Write(vrfBytes)
	hv := hs.Sum(nil)
	hash := new(big.Int)
	hash.SetBytes(hv[:])
	n := big.NewInt(0)
	for _, validator := range validators {
		n.Add(n, validator.VotingPower)
	}
	n.Mod(hash, n)

	for i, validator := range validators {
		n.Sub(n, validator.VotingPower)
		if n.Sign() == -1 {
			idx = i
			break
		}
	}

	return idx
}

func (cs *ConsensusState) GetProposer() *types.Validator {

	if cs.proposer == nil || cs.proposer.Proposer == nil || cs.Height != cs.proposer.Height || cs.Round != cs.proposer.Round {
		cs.updateProposer()
	}
	return cs.proposer.Proposer
}

func (cs *ConsensusState) IsProposer() bool {

	cs.GetProposer()
	return cs.isProposer
}

func (cs *ConsensusState) SetTimeoutTicker(timeoutTicker TimeoutTicker) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.timeoutTicker = timeoutTicker
}

func (cs *ConsensusState) LoadCommit(height uint64) *types.Commit {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	ncExtra, height := cs.LoadNeatConExtra(height)
	return ncExtra.SeenCommit
}

func (cs *ConsensusState) OnStart() error {

	cs.done = make(chan struct{})

	cs.timeoutTicker.Start()

	go cs.receiveRoutine(0)

	cs.StartNewHeight()

	return nil
}

func (cs *ConsensusState) OnStop() {

	cs.BaseService.OnStop()
	cs.timeoutTicker.Stop()
}

func (cs *ConsensusState) Wait() {
	<-cs.done
}

func (cs *ConsensusState) AddVote(vote *types.Vote, peerKey string) (added bool, err error) {
	if peerKey == "" {
		cs.internalMsgQueue <- msgInfo{&VoteMessage{vote}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&VoteMessage{vote}, peerKey}
	}

	return false, nil
}

func (cs *ConsensusState) SetProposal(proposal *types.Proposal, peerKey string) error {

	if peerKey == "" {
		cs.internalMsgQueue <- msgInfo{&ProposalMessage{proposal}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&ProposalMessage{proposal}, peerKey}
	}

	return nil
}

func (cs *ConsensusState) AddProposalBlockPart(height uint64, round int, part *types.Part, peerKey string) error {

	if peerKey == "" {
		cs.internalMsgQueue <- msgInfo{&BlockPartMessage{height, round, part}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&BlockPartMessage{height, round, part}, peerKey}
	}

	return nil
}

func (cs *ConsensusState) SetProposalAndBlock(proposal *types.Proposal, block *types.NCBlock, parts *types.PartSet, peerKey string) error {
	cs.SetProposal(proposal, peerKey)
	for i := 0; i < parts.Total(); i++ {
		part := parts.GetPart(i)
		cs.AddProposalBlockPart(proposal.Height, proposal.Round, part, peerKey)
	}
	return nil
}

func (cs *ConsensusState) updateRoundStep(round int, step RoundStepType) {
	cs.Round = round
	cs.Step = step
}

func (cs *ConsensusState) scheduleRound0(rs *RoundState) {
	sleepDuration := rs.StartTime.Sub(time.Now())
	cs.scheduleTimeout(sleepDuration, rs.Height, 0, RoundStepNewHeight)
}

func (cs *ConsensusState) scheduleTimeout(duration time.Duration, height uint64, round int, step RoundStepType) {
	cs.timeoutTicker.ScheduleTimeout(timeoutInfo{duration, height, round, step})
}

func (cs *ConsensusState) sendInternalMessage(mi msgInfo) {
	select {
	case cs.internalMsgQueue <- mi:
	default:
		cs.logger.Warn("Internal msg queue is full. Using a go-routine")
		go func() { cs.internalMsgQueue <- mi }()
	}
}

func (cs *ConsensusState) ReconstructLastCommit(state *sm.State) {

	state.NTCExtra, _ = cs.LoadLastNeatConExtra()
	if state.NTCExtra == nil {
		return
	}
}

func (cs *ConsensusState) newStep() {
	rs := cs.RoundStateEvent()

	cs.nSteps += 1
	if cs.evsw != nil {
		types.FireEventNewRoundStep(cs.evsw, rs)
	}
}

func (cs *ConsensusState) receiveRoutine(maxSteps int) {
	for {
		if maxSteps > 0 {
			if cs.nSteps >= maxSteps {
				cs.logger.Warn("receiveRoutine. reached max steps. exiting receive routine")
				cs.nSteps = 0
				return
			}
		}
		var mi msgInfo

		select {
		case mi = <-cs.peerMsgQueue:
			rs := cs.RoundState
			cs.handleMsg(mi, rs)
		case mi = <-cs.internalMsgQueue:
			rs := cs.RoundState
			cs.handleMsg(mi, rs)
		case ti := <-cs.timeoutTicker.Chan():
			rs := cs.RoundState
			cs.handleTimeout(ti, rs)
		case <-cs.Quit:
			close(cs.done)
			return
		}
	}
}

func (cs *ConsensusState) handleMsg(mi msgInfo, rs RoundState) {

	var err error
	msg, peerKey := mi.Msg, mi.PeerKey
	switch msg := msg.(type) {
	case *ProposalMessage:
		cs.logger.Debugf("handleMsg: Received proposal message %v", msg.Proposal)
		cs.mtx.Lock()
		err = cs.setProposal(msg.Proposal)
		cs.mtx.Unlock()
	case *BlockPartMessage:
		cs.mtx.Lock()
		_, err = cs.addProposalBlockPart(msg.Height, msg.Round, msg.Part, peerKey != "")
		if err != nil && msg.Round != cs.Round {
			err = nil
		}
		cs.mtx.Unlock()
	case *Maj23SignAggrMessage:
		cs.mtx.Lock()
		err = cs.handleSignAggr(msg.Maj23SignAggr)
		cs.mtx.Unlock()
	case *VoteMessage:
		cs.mtx.Lock()
		err := cs.tryAddVote(msg.Vote, peerKey)
		cs.mtx.Unlock()
		if err == ErrAddingVote {
		}

	default:
		cs.logger.Warnf("handleMsg. Unknown msg type %v", reflect.TypeOf(msg))
	}

	if err != nil {
		cs.logger.Errorf("handleMsg. msg: %v, error: %v", msg, err)
	}
}

func (cs *ConsensusState) handleTimeout(ti timeoutInfo, rs RoundState) {

	if ti.Height != rs.Height || ti.Round < rs.Round || (ti.Round == rs.Round && ti.Step < rs.Step) {
		cs.logger.Warn("Ignoring tock because we're ahead")
		return
	}

	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.logger.Debugf("step is :%+v", ti.Step)
	switch ti.Step {
	case RoundStepNewHeight:
		cs.enterNewRound(ti.Height, 0)
	case RoundStepWaitForMinerBlock:
		types.FireEventTimeoutPropose(cs.evsw, cs.RoundStateEvent())
		if cs.blockFromMiner != nil {
			cs.logger.Warn("another round of RoundStepWaitForMinerBlock, something wrong!!!")
		}
		cs.enterPropose(ti.Height, ti.Round)
	case RoundStepPropose:
		types.FireEventTimeoutPropose(cs.evsw, cs.RoundStateEvent())
		cs.enterPrevote(ti.Height, ti.Round)
	case RoundStepPrevoteWait:
		types.FireEventTimeoutWait(cs.evsw, cs.RoundStateEvent())
		cs.enterPrecommit(ti.Height, ti.Round)
	case RoundStepPrecommitWait:
		types.FireEventTimeoutWait(cs.evsw, cs.RoundStateEvent())
		cs.enterNewRound(ti.Height, ti.Round+1)
	default:
		panic(Fmt("Invalid timeout step: %v", ti.Step))
	}
}

func (cs *ConsensusState) enterNewRound(height uint64, round int) {
	if cs.Height != height || round < cs.Round || (cs.Round == round && cs.Step != RoundStepNewHeight) {
		cs.logger.Warnf("enterNewRound(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step)
		return
	}

	if now := time.Now(); cs.StartTime.After(now) {
		cs.logger.Warn("Need to set a buffer and log.Warn() here for sanity.", "startTime", cs.StartTime, "now", now)
	}

	cs.updateRoundStep(round, RoundStepNewRound)
	if round == 0 {
	} else {
		cs.Proposal = nil
		cs.ProposalBlock = nil
		cs.ProposalBlockParts = nil
		cs.ProposerPeerKey = ""
		cs.PrevoteMaj23SignAggr = nil
		cs.PrecommitMaj23SignAggr = nil
	}
	cs.VoteSignAggr.SetRound(round + 1)
	cs.Votes.SetRound(round + 1)
	cs.pastRoundStates[round] = ROUND_NOT_PROPOSED
	types.FireEventNewRound(cs.evsw, cs.RoundStateEvent())

	if cs.IsProposer() && (cs.blockFromMiner == nil || cs.Height != cs.blockFromMiner.NumberU64()) {

		if cs.blockFromMiner == nil {
			cs.logger.Info("we are proposer, but blockFromMiner is nil , let's wait a second!!!")
		} else {
			cs.logger.Info("we are proposer, but height mismatch",
				"cs.Height", cs.Height, "cs.blockFromMiner.NumberU64()", cs.blockFromMiner.NumberU64())
		}
		cs.scheduleTimeout(cs.timeoutParams.WaitForMinerBlock(), height, round, RoundStepWaitForMinerBlock)
		return
	}

	cs.enterPropose(height, round)
}

func (cs *ConsensusState) enterLowerRound(height uint64, round int) {

	if now := time.Now(); cs.StartTime.After(now) {
		cs.logger.Warn("Need to set a buffer and log.Warn() here for sanity.", "startTime", cs.StartTime, "now", now)
	}

	for r := cs.Round; r > round; r-- {
		if _, ok := cs.pastRoundStates[r]; ok {
			delete(cs.pastRoundStates, r)
		}
	}

	cs.updateRoundStep(round, RoundStepNewRound)

	cs.Proposal = nil
	cs.ProposalBlock = nil
	cs.ProposalBlockParts = nil
	cs.ProposerPeerKey = ""
	cs.PrevoteMaj23SignAggr = nil
	cs.PrecommitMaj23SignAggr = nil

	cs.VoteSignAggr.ResetTop2Round(round + 1)
	cs.Votes.ResetTop2Round(round + 1)
	types.FireEventNewRound(cs.evsw, cs.RoundStateEvent())

	if cs.IsProposer() && (cs.blockFromMiner == nil || cs.Height != cs.blockFromMiner.NumberU64()) {

		if cs.blockFromMiner == nil {
			cs.logger.Info("we are proposer, but blockFromMiner is nil , let's wait a second!!!")
		} else {
			cs.logger.Info("we are proposer, but height mismatch",
				"cs.Height", cs.Height, "cs.blockFromMiner.NumberU64()", cs.blockFromMiner.NumberU64())
		}
		cs.scheduleTimeout(cs.timeoutParams.WaitForMinerBlock(), height, round, RoundStepWaitForMinerBlock)
		return
	}

	cs.enterPropose(height, round)
}

func (cs *ConsensusState) enterPropose(height uint64, round int) {
	if cs.Height != height || round < cs.Round || (cs.Round == round && RoundStepPropose <= cs.Step) {

		return
	}

	defer func() {

		cs.updateRoundStep(round, RoundStepPropose)
		cs.newStep()

		if cs.isProposalComplete() {
			cs.enterPrevote(height, cs.Round)
		}
	}()

	if cs.state.NTCExtra.NeedToSave &&
		(cs.state.NTCExtra.ChainID != params.MainnetChainConfig.NeatChainId && cs.state.NTCExtra.ChainID != params.TestnetChainConfig.NeatChainId) {
		if cs.privValidator != nil && cs.IsProposer() {
			cs.logger.Infof("enterPropose: saveBlockToMainChain height: %v", cs.state.NTCExtra.Height)
			lastBlock := cs.GetChainReader().GetBlockByNumber(cs.state.NTCExtra.Height)
			cs.saveBlockToMainChain(lastBlock)
			cs.state.NTCExtra.NeedToSave = false
		}
	}

	if cs.state.NTCExtra.NeedToBroadcast &&
		(cs.state.NTCExtra.ChainID != params.MainnetChainConfig.NeatChainId && cs.state.NTCExtra.ChainID != params.TestnetChainConfig.NeatChainId) {
		if cs.privValidator != nil && cs.IsProposer() {
			cs.logger.Infof("enterPropose: broadcastTX3ProofDataToMainChain height: %v", cs.state.NTCExtra.Height)
			lastBlock := cs.GetChainReader().GetBlockByNumber(cs.state.NTCExtra.Height)
			cs.broadcastTX3ProofDataToMainChain(lastBlock)
			cs.state.NTCExtra.NeedToBroadcast = false
		}
	}

	cs.scheduleTimeout(cs.timeoutParams.Propose(round), height, round, RoundStepPropose)

	if cs.privValidator == nil {
		cs.logger.Info("you are not validator yet!!!")
		return
	}

	if !cs.IsProposer() {

	} else {

		cs.decideProposal(height, round)
	}
}

func (cs *ConsensusState) defaultDecideProposal(height uint64, round int) {
	var block *types.NCBlock
	var blockParts *types.PartSet
	var proposerPeerKey string

	if cs.LockedBlock != nil {

		block, blockParts = cs.LockedBlock, cs.LockedBlockParts
	} else {

		block, blockParts = cs.createProposalBlock()
		if block == nil {
			return
		}
	}

	polRound, polBlockID := cs.VoteSignAggr.POLInfo()
	cs.logger.Debugf("proposal hash: %X", block.Hash())
	if NodeID == "" {
		panic("Node id is nil")
	}
	proposerPeerKey = NodeID

	cs.logger.Debugf("defaultDecideProposal: Proposer (peer key %s)", proposerPeerKey)

	proposal := types.NewProposal(height, round, block.Hash(), blockParts.Header(), polRound, polBlockID, proposerPeerKey)
	err := cs.privValidator.SignProposal(cs.state.NTCExtra.ChainID, proposal)
	if err == nil {

		cs.sendInternalMessage(msgInfo{&ProposalMessage{proposal}, ""})
		for i := 0; i < blockParts.Total(); i++ {
			part := blockParts.GetPart(i)
			cs.sendInternalMessage(msgInfo{&BlockPartMessage{cs.Height, cs.Round, part}, ""})
		}
	} else {
		log.Warn("enterPropose: Error signing proposal", "height", height, "round", round, "error", err)
	}
}

func (cs *ConsensusState) isProposalComplete() bool {

	if cs.Proposal == nil || cs.ProposalBlock == nil {
		return false
	}

	return true
}

func (cs *ConsensusState) createProposalBlock() (*types.NCBlock, *types.PartSet) {

	if cs.blockFromMiner != nil {

		if cs.Height != cs.blockFromMiner.NumberU64() {
			log.Warn("createProposalBlock(), height mismatch", "cs.Height", cs.Height,
				"cs.blockFromMiner.NumberU64()", cs.blockFromMiner.NumberU64())
			return nil, nil
		}

		neatBlock := cs.blockFromMiner

		var commit = &types.Commit{}
		var epochBytes []byte

		if cs.Height == cs.Epoch.EndBlock {

			nextEp := cs.Epoch.GetNextEpoch()
			if nextEp == nil {
				panic("missing next epoch at epoch end block")
			}
			epochBytes = nextEp.Bytes()
		} else if cs.Height == cs.Epoch.StartBlock || cs.Height == 1 {

			epochBytes = cs.Epoch.Bytes()
		} else {
			shouldProposeEpoch := cs.Epoch.ShouldProposeNextEpoch(cs.Height)

			if shouldProposeEpoch {
				lastHeight := cs.backend.ChainReader().CurrentBlock().Number().Uint64()
				lastBlockTime := time.Unix(int64(cs.backend.ChainReader().CurrentBlock().Time()), 0)
				epochBytes = cs.Epoch.ProposeNextEpoch(lastHeight, lastBlockTime).Bytes()
			}
		}

		_, val, _ := cs.state.GetValidators()

		var tx3ProofData []*neatTypes.TX3ProofData
		txs := neatBlock.Transactions()
		for _, tx := range txs {
			if neatAbi.IsNeatChainContractAddr(tx.To()) {
				data := tx.Data()
				function, err := neatAbi.FunctionTypeFromId(data[:4])
				if err != nil {
					continue
				}

				if function == neatAbi.WithdrawFromMainChain {
					var args neatAbi.WithdrawFromMainChainArgs
					data := tx.Data()
					if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.WithdrawFromMainChain.String(), data[4:]); err != nil {
						continue
					}

					proof := cs.cch.GetTX3ProofData(args.ChainId, args.TxHash)
					if proof != nil {
						tx3ProofData = append(tx3ProofData, proof)
					}
				}
			}
		}

		return types.MakeBlock(cs.Height, cs.state.NTCExtra.ChainID, commit, neatBlock,
			val.Hash(), cs.Epoch.Number, epochBytes,
			tx3ProofData, 65536)
	} else {
		cs.logger.Warn("block from miner should not be nil, let's start another round")
		return nil, nil
	}
}

func (cs *ConsensusState) enterPrevote(height uint64, round int) {
	if cs.Height != height || round < cs.Round || (cs.Round == round && RoundStepPrevoteWait < cs.Step) {
		cs.logger.Warnf("enterPrevote(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step)
		return
	}

	defer func() {

		if cs.Step == RoundStepPropose {
			cs.updateRoundStep(round, RoundStepPrevote)
			cs.newStep()
		}

		cs.enterPrevoteWait(height, round)
	}()

	if cs.isProposalComplete() {
		cs.doPrevote(height, round)
	}

}

func (cs *ConsensusState) defaultDoPrevote(height uint64, round int) {

	if cs.LockedBlock != nil {

		cs.signAddVote(types.VoteTypePrevote, cs.LockedBlock.Hash(), cs.LockedBlockParts.Header())
		return
	}

	if cs.ProposalBlock == nil {
		cs.logger.Warn("enterPrevote: ProposalBlock is nil")
		cs.signAddVote(types.VoteTypePrevote, nil, types.PartSetHeader{})
		return
	}

	err := cs.ProposalBlock.ValidateBasic(cs.state.NTCExtra)
	if err != nil {

		cs.logger.Warnf("enterPrevote: ProposalBlock is invalid, error: %v", err)
		cs.signAddVote(types.VoteTypePrevote, nil, types.PartSetHeader{})
		return
	}

	err = cs.ValidateTX4(cs.ProposalBlock)
	if err != nil {

		cs.logger.Warnf("enterPrevote: ProposalBlock is invalid, error: %v", err)
		cs.signAddVote(types.VoteTypePrevote, nil, types.PartSetHeader{})
		return
	}

	if !cs.IsProposer() {
		if cv, ok := cs.backend.ChainReader().(consss.ChainValidator); ok {

			state, receipts, ops, err := cv.ValidateBlock(cs.ProposalBlock.Block)
			if err != nil {

				cs.logger.Warnf("enterPrevote: ValidateBlock fail, error: %v", err)
				cs.signAddVote(types.VoteTypePrevote, nil, types.PartSetHeader{})
				return
			}

			cs.ProposalBlock.IntermediateResult = &types.IntermediateBlockResult{
				State:    state,
				Receipts: receipts,
				Ops:      ops,
			}
		}
	}

	proposedNextEpoch := ep.FromBytes(cs.ProposalBlock.NTCExtra.EpochBytes)
	if proposedNextEpoch != nil && proposedNextEpoch.Number == cs.Epoch.Number+1 {
		if cs.Epoch.ShouldProposeNextEpoch(cs.Height) {
			lastHeight := cs.backend.ChainReader().CurrentBlock().Number().Uint64()
			lastBlockTime := time.Unix(int64(cs.backend.ChainReader().CurrentBlock().Time()), 0)
			err = cs.Epoch.ValidateNextEpoch(proposedNextEpoch, lastHeight, lastBlockTime)
			if err != nil {

				cs.logger.Warnf("enterPrevote: Proposal Next Epoch is invalid, error: %v", err)
				cs.signAddVote(types.VoteTypePrevote, nil, types.PartSetHeader{})
				return
			}
		}
	}

	cs.signAddVote(types.VoteTypePrevote, cs.ProposalBlock.Hash(), cs.ProposalBlockParts.Header())
	return
}

func (cs *ConsensusState) enterPrevoteWait(height uint64, round int) {
	if cs.Height != height || round < cs.Round || (cs.Round == round && RoundStepPrevoteWait <= cs.Step) {
		cs.logger.Warnf("enterPrevoteWait(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step)
		return
	}

	defer func() {

		cs.updateRoundStep(round, RoundStepPrevoteWait)
		cs.newStep()
	}()

	cs.scheduleTimeout(cs.timeoutParams.Prevote(round), height, round, RoundStepPrevoteWait)
}

func (cs *ConsensusState) enterPrecommit(height uint64, round int) {
	if cs.Height != height || round < cs.Round || (cs.Round == round && RoundStepPrecommit <= cs.Step) {

		return
	}

	defer func() {

		cs.updateRoundStep(round, RoundStepPrecommit)
		cs.newStep()

		cs.enterPrecommitWait(height, round)
	}()

	blockID, ok := cs.VoteSignAggr.Prevotes(round).TwoThirdsMajority()

	if !ok {

		cs.backend.GetBroadcaster().TryFixBadPreimages()

		if cs.LockedBlock != nil {

		} else {

		}
		cs.signAddVote(types.VoteTypePrecommit, nil, types.PartSetHeader{})
		return
	}

	types.FireEventPolka(cs.evsw, cs.RoundStateEvent())

	polRound, _ := cs.VoteSignAggr.POLInfo()
	if polRound < round {
		PanicSanity(Fmt("This POLRound should be %v but got %", round, polRound))
	}

	if len(blockID.Hash) == 0 {
		if cs.LockedBlock == nil {

		} else {

			cs.LockedRound = -1
			cs.LockedBlock = nil
			cs.LockedBlockParts = nil
			types.FireEventUnlock(cs.evsw, cs.RoundStateEvent())
		}
		cs.signAddVote(types.VoteTypePrecommit, nil, types.PartSetHeader{})
		return
	}

	if cs.LockedBlock.HashesTo(blockID.Hash) {

		cs.LockedRound = round
		types.FireEventRelock(cs.evsw, cs.RoundStateEvent())
		cs.signAddVote(types.VoteTypePrecommit, blockID.Hash, blockID.PartsHeader)
		return
	}

	cs.logger.Debugf("cs proposal hash:%+v", cs.ProposalBlock.Hash())
	cs.logger.Debugf("block id:%+v", blockID.Hash)

	if cs.ProposalBlock.HashesTo(blockID.Hash) {

		if err := cs.ProposalBlock.ValidateBasic(cs.state.NTCExtra); err != nil {
			PanicConsensus(Fmt("enterPrecommit: +2/3 prevoted for an invalid block: %v", err))
		}
		cs.LockedRound = round
		cs.LockedBlock = cs.ProposalBlock
		cs.LockedBlockParts = cs.ProposalBlockParts
		types.FireEventLock(cs.evsw, cs.RoundStateEvent())
		cs.signAddVote(types.VoteTypePrecommit, blockID.Hash, blockID.PartsHeader)
		return
	}

	cs.LockedRound = -1
	cs.LockedBlock = nil
	cs.LockedBlockParts = nil
	if !cs.ProposalBlockParts.HasHeader(blockID.PartsHeader) {
		cs.ProposalBlock = nil
		cs.ProposalBlockParts = types.NewPartSetFromHeader(blockID.PartsHeader)
	}
	types.FireEventUnlock(cs.evsw, cs.RoundStateEvent())
	cs.signAddVote(types.VoteTypePrecommit, nil, types.PartSetHeader{})
	return
}

func (cs *ConsensusState) enterPrecommitWait(height uint64, round int) {
	if cs.Height != height || round < cs.Round || (cs.Round == round && RoundStepPrecommitWait <= cs.Step) {

		return
	}

	defer func() {

		cs.updateRoundStep(round, RoundStepPrecommitWait)
		cs.newStep()
	}()

	cs.scheduleTimeout(cs.timeoutParams.Precommit(round), height, round, RoundStepPrecommitWait)

}

func (cs *ConsensusState) enterCommit(height uint64, commitRound int) {
	if cs.Height != height || RoundStepCommit <= cs.Step {
		cs.logger.Warnf("enterCommit(%v/%v): Invalid args. Current step: %v/%v/%v", height, commitRound, cs.Height, cs.Round, cs.Step)
		return
	}

	defer func() {

		cs.updateRoundStep(cs.Round, RoundStepCommit)
		cs.CommitRound = commitRound
		cs.CommitTime = time.Now()
		cs.newStep()

		cs.tryFinalizeCommit(height)
	}()

	blockID, ok := cs.VoteSignAggr.Precommits(commitRound).TwoThirdsMajority()
	if !ok {
		PanicSanity("RunActionCommit() expects +2/3 precommits")
	}

	if cs.LockedBlock.HashesTo(blockID.Hash) {
		cs.ProposalBlock = cs.LockedBlock
		cs.ProposalBlockParts = cs.LockedBlockParts
	}

	if !cs.ProposalBlock.HashesTo(blockID.Hash) {
		if !cs.ProposalBlockParts.HasHeader(blockID.PartsHeader) {

			cs.ProposalBlock = nil
			cs.ProposalBlockParts = types.NewPartSetFromHeader(blockID.PartsHeader)
		} else {

		}
	}

}

func (cs *ConsensusState) tryFinalizeCommit(height uint64) {

	if cs.Height != height {
		PanicSanity(Fmt("tryFinalizeCommit() cs.Height: %v vs height: %v", cs.Height, height))
	}

	blockID, ok := cs.VoteSignAggr.Precommits(cs.CommitRound).TwoThirdsMajority()
	if !ok || len(blockID.Hash) == 0 {
		cs.logger.Warn("Attempt to finalize failed. There was no +2/3 majority, or +2/3 was for <nil>.", "height", height)
		return
	}
	if !cs.ProposalBlock.HashesTo(blockID.Hash) {

		cs.logger.Warn("Attempt to finalize failed. We don't have the commit block.", "height", height, "proposal-block", cs.ProposalBlock.Hash(), "commit-block", blockID.Hash)
		return
	}

	cs.finalizeCommit(height)
}

func (cs *ConsensusState) finalizeCommit(height uint64) {
	if cs.Height != height || cs.Step != RoundStepCommit {
		cs.logger.Warnf("finalizeCommit(%v): Invalid args. Current step: %v/%v/%v", height, cs.Height, cs.Round, cs.Step)
		return
	}

	blockID, ok := cs.VoteSignAggr.Precommits(cs.CommitRound).TwoThirdsMajority()
	block, blockParts := cs.ProposalBlock, cs.ProposalBlockParts

	if !ok {
		PanicSanity(Fmt("Cannot finalizeCommit, commit does not have two thirds majority"))
	}
	if !blockParts.HasHeader(blockID.PartsHeader) {
		PanicSanity(Fmt("Expected ProposalBlockParts header to be commit header"))
	}
	if !block.HashesTo(blockID.Hash) {
		PanicSanity(Fmt("Cannot finalizeCommit, ProposalBlock does not hash to commit hash"))
	}
	if err := block.ValidateBasic(cs.state.NTCExtra); err != nil {
		PanicConsensus(Fmt("+2/3 committed an invalid block: %v", err))
	}

	if cs.state.NTCExtra.Height < block.NTCExtra.Height {

		precommits := cs.VoteSignAggr.Precommits(cs.CommitRound)
		seenCommit := precommits.MakeCommit()

		block.NTCExtra.SeenCommit = seenCommit
		block.NTCExtra.SeenCommitHash = seenCommit.Hash()

		if block.NTCExtra.ChainID != params.MainnetChainConfig.NeatChainId && block.NTCExtra.ChainID != params.TestnetChainConfig.NeatChainId {

			if len(block.NTCExtra.EpochBytes) > 0 {
				block.NTCExtra.NeedToSave = true
				cs.logger.Infof("NeedToSave set to true due to epoch. Chain: %s, Height: %v", block.NTCExtra.ChainID, block.NTCExtra.Height)
			}

			txs := block.Block.Transactions()
			for _, tx := range txs {
				if neatAbi.IsNeatChainContractAddr(tx.To()) {
					data := tx.Data()
					function, err := neatAbi.FunctionTypeFromId(data[:4])
					if err != nil {
						continue
					}

					if function == neatAbi.WithdrawFromSideChain {
						block.NTCExtra.NeedToBroadcast = true
						cs.logger.Infof("NeedToBroadcast set to true due to tx. Tx: %s, Chain: %s, Height: %v", function.String(), block.NTCExtra.ChainID, block.NTCExtra.Height)
						break
					}
				}
			}
		}

		types.FireEventNewBlock(cs.evsw, types.EventDataNewBlock{block})
		types.FireEventNewBlockHeader(cs.evsw, types.EventDataNewBlockHeader{int(block.NTCExtra.Height)})

		err := cs.backend.Commit(block, [][]byte{}, cs.IsProposer)
		if err != nil {
			cs.logger.Errorf("Commit fail. error: %v", err)
		}
	} else {
		cs.logger.Warn("Calling finalizeCommit on already stored block", "height", block.NTCExtra.Height)
	}

	return
}

func (cs *ConsensusState) defaultSetProposal(proposal *types.Proposal) error {

	if cs.Proposal != nil {
		return nil
	}

	if proposal.Height != cs.Height || proposal.Round > cs.Round {
		return nil
	}

	if RoundStepCommit <= cs.Step {
		return nil
	}

	if proposal.POLRound != -1 &&
		(proposal.POLRound < 0 || proposal.Round <= proposal.POLRound) {
		return ErrInvalidProposalPOLRound
	}

	if proposal.Round == cs.Round {

		if !cs.GetProposer().PubKey.VerifyBytes(types.SignBytes(cs.chainConfig.NeatChainId, proposal), proposal.Signature) {
			return ErrInvalidProposalSignature
		}

	} else {

		if !cs.proposerByRound(proposal.Round).Proposer.PubKey.VerifyBytes(types.SignBytes(cs.chainConfig.NeatChainId, proposal), proposal.Signature) {
			return ErrInvalidProposalSignature
		}

		if len(cs.pastRoundStates)-1 < proposal.Round {
			cs.logger.Infof("length of pastRoundStates less then proposal.Round, not possible")
			return nil
		}

		if cs.pastRoundStates[proposal.Round] != ROUND_NOT_PROPOSED {
			cs.logger.Infof("pastRoundStates in proposal.Round %x proposed, past", proposal.Round)
			return nil
		}

		cs.enterLowerRound(cs.Height, proposal.Round)
	}

	cs.Proposal = proposal
	cs.logger.Debugf("proposal is: %X", proposal.Hash)
	cs.ProposalBlockParts = types.NewPartSetFromHeader(proposal.BlockPartsHeader)
	cs.ProposerPeerKey = proposal.ProposerPeerKey

	cs.pastRoundStates[cs.Round] = ROUND_PROPOSED

	return nil
}

func (cs *ConsensusState) addProposalBlockPart(height uint64, round int, part *types.Part, verify bool) (added bool, err error) {

	if cs.Height != height || cs.Round != round {
		return false, nil
	}

	if cs.ProposalBlockParts == nil {
		return false, nil
	}

	added, err = cs.ProposalBlockParts.AddPart(part, verify)
	if err != nil {
		return added, err
	}
	if added && cs.ProposalBlockParts.IsComplete() {

		ncBlock := &types.NCBlock{}
		cs.ProposalBlock, err = ncBlock.FromBytes(cs.ProposalBlockParts.GetReader())

		if RoundStepPropose <= cs.Step && cs.Step <= RoundStepPrevoteWait && cs.isProposalComplete() {

			cs.enterPrevote(height, cs.Round)
		} else if cs.Step == RoundStepCommit {

			cs.tryFinalizeCommit(height)
		}

		return true, err
	}
	return added, nil
}

func (cs *ConsensusState) setMaj23SignAggr(signAggr *types.SignAggr) (error, bool) {
	cs.logger.Debug("enter setMaj23SignAggr()")
	cs.logger.Debugf("Received SignAggr %#v", signAggr)

	if signAggr.Height != cs.Height {
		cs.logger.Debug("does not apply for this height")
		return nil, false
	}

	if signAggr.SignAggr() == nil {
		cs.logger.Debug("SignAggr() is nil ")
	}
	maj23, err := cs.blsVerifySignAggr(signAggr)

	if err != nil || maj23 == false {
		cs.logger.Warnf("verifyMaj23SignAggr: Invalid signature aggregation, error:%+v, maj23:%+v", err, maj23)
		cs.logger.Warnf("SignAggr:%+v", signAggr)
		return ErrInvalidSignatureAggr, false
	}

	if signAggr.Type == types.VoteTypePrevote ||
		signAggr.Type == types.VoteTypePrecommit {

		cs.VoteSignAggr.AddSignAggr(signAggr)
	} else {

		cs.logger.Warn(Fmt("setMaj23SignAggr: invalid type %d for signAggr %#v\n", signAggr.Type, signAggr))
		return ErrInvalidSignatureAggr, false
	}

	if signAggr.Round != cs.Round {
		cs.logger.Debug("does not apply for this round")
		return nil, false
	}

	if signAggr.Type == types.VoteTypePrevote {

		if cs.PrevoteMaj23SignAggr != nil {
			return ErrDuplicateSignatureAggr, false
		}

		cs.PrevoteMaj23SignAggr = signAggr

		if (cs.LockedBlock != nil) && (cs.LockedRound < signAggr.Round) {
			blockID := cs.PrevoteMaj23SignAggr.Maj23
			if !cs.LockedBlock.HashesTo(blockID.Hash) {
				cs.logger.Info("Unlocking because of POL.", "lockedRound", cs.LockedRound, "POLRound", signAggr.Round)
				cs.LockedRound = -1
				cs.LockedBlock = nil
				cs.LockedBlockParts = nil
				types.FireEventUnlock(cs.evsw, cs.RoundStateEvent())
			}
		}

		cs.logger.Debugf("setMaj23SignAggr:prevote aggr %#v", cs.PrevoteMaj23SignAggr)
	} else if signAggr.Type == types.VoteTypePrecommit {
		if cs.PrecommitMaj23SignAggr != nil {
			return ErrDuplicateSignatureAggr, false
		}

		cs.PrecommitMaj23SignAggr = signAggr
		cs.logger.Debugf("setMaj23SignAggr:precommit aggr %#v", cs.PrecommitMaj23SignAggr)
	}

	if signAggr.Type == types.VoteTypePrevote {

		if cs.isProposalComplete() {
			cs.logger.Debugf("receive block:%+v", cs.ProposalBlock)
			cs.enterPrecommit(cs.Height, cs.Round)
			return nil, true

		} else {
			cs.logger.Debug("block is not completed")
			return nil, false
		}
	} else if signAggr.Type == types.VoteTypePrecommit {

		if cs.isProposalComplete() {
			cs.logger.Debug("block is completed")

			cs.enterCommit(cs.Height, cs.Round)
			return nil, true
		} else {
			cs.logger.Debug("block is not completed")
			return nil, false
		}

	} else {
		panic("Invalid signAggr type")
		return nil, false
	}
	return nil, false
}

func (cs *ConsensusState) handleSignAggr(signAggr *types.SignAggr) error {
	if signAggr == nil {
		return fmt.Errorf("SignAggr is nil")
	}
	if signAggr.Height == cs.Height {
		err, _ := cs.setMaj23SignAggr(signAggr)
		return err
	}
	return nil
}

func (cs *ConsensusState) BLSVerifySignAggr(signAggr *types.SignAggr) (bool, error) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	return cs.blsVerifySignAggr(signAggr)
}

func (cs *ConsensusState) blsVerifySignAggr(signAggr *types.SignAggr) (bool, error) {
	cs.logger.Debug("enter BLSVerifySignAggr()")
	cs.logger.Debugf("sign aggr bitmap:%+v", signAggr.BitArray)
	if signAggr == nil {
		cs.logger.Info("Invalid Sign(nil)")
		return false, fmt.Errorf("Invalid SignAggr(nil)")
	}

	if signAggr.SignAggr() == nil {
		cs.logger.Info("Invalid BLSSignature(nil)")
		return false, fmt.Errorf("Invalid BLSSignature(nil)")
	}

	bitMap := signAggr.BitArray
	validators := cs.Validators

	_, votesSum, totalVotes, err := validators.TalliedVotingPower(bitMap)
	if err != nil {
		cs.logger.Info("tallied voting power")
		return false, err
	}

	quorum := types.Loose23MajorThreshold(totalVotes, signAggr.Round)

	var maj23 bool

	if votesSum.Cmp(quorum) >= 0 {
		maj23 = true
	} else {
		maj23 = false
	}

	aggrPubKey := validators.AggrPubKey(bitMap)
	if aggrPubKey == nil {
		cs.logger.Info("can not aggregate pubkeys")
		return false, fmt.Errorf("can not aggregate pubkeys")
	}

	vote := &types.Vote{
		BlockID: signAggr.BlockID,
		Height:  signAggr.Height,
		Round:   (uint64)(signAggr.Round),
		Type:    signAggr.Type,
	}

	if !aggrPubKey.VerifyBytes(types.SignBytes(signAggr.ChainID, vote), signAggr.SignAggr()) {
		cs.logger.Info("Invalid aggregate signature")
		return false, errors.New("Invalid aggregate signature")
	}

	return maj23, nil
}

func (cs *ConsensusState) tryAddVote(vote *types.Vote, peerKey string) error {
	_, err := cs.addVote(vote, peerKey)
	if err != nil {

		if err == ErrVoteHeightMismatch {
			return err
		} else if _, ok := err.(*types.ErrVoteConflictingVotes); ok {
			if peerKey == "" {
				cs.logger.Warn("Found conflicting vote from ourselves. Did you unsafe_reset a validator?", "height", vote.Height, "round", vote.Round, "type", vote.Type)
				return err
			}
			return err
		} else {

			cs.logger.Warn("Error attempting to add vote", "error", err)
			return ErrAddingVote
		}
	}
	return nil
}

func (cs *ConsensusState) addVote(vote *types.Vote, peerKey string) (added bool, err error) {

	if !cs.IsProposer() {
		cs.logger.Warn("addVode should only happen if this node is proposer")
		return
	}

	if vote.Height != cs.Height || int(vote.Round) != cs.Round {
		cs.logger.Warn("addVote, vote is for previous blocks or previous round, just ignore\n")
		return
	}

	if vote.Type == types.VoteTypePrevote {
		if cs.Votes.Prevotes(cs.Round).HasTwoThirdsMajority() {
			return
		}
	} else {
		if cs.Votes.Precommits(cs.Round).HasTwoThirdsMajority() {
			return
		}
	}

	added, err = cs.Votes.AddVote(vote, peerKey)
	if added {
		if vote.Type == types.VoteTypePrevote {

			if cs.Votes.Prevotes(cs.Round).HasTwoThirdsMajority() {
				cs.logger.Debug(Fmt("addVote: Got 2/3+ prevotes %+v\n", cs.Votes.Prevotes(cs.Round)))

				cs.sendMaj23SignAggr(vote.Type)
			}
		} else if vote.Type == types.VoteTypePrecommit {
			if cs.Votes.Precommits(cs.Round).HasTwoThirdsMajority() {
				cs.logger.Debugf("addVote: Got 2/3+ precommits %+v", cs.Votes.Precommits(cs.Round))

				cs.sendMaj23SignAggr(vote.Type)
			}
		}
	}

	return
}

func (cs *ConsensusState) signVote(type_ byte, hash []byte, header types.PartSetHeader) (*types.Vote, error) {
	addr := cs.privValidator.GetAddress()
	valIndex, _ := cs.Validators.GetByAddress(addr)
	vote := &types.Vote{
		ValidatorAddress: addr,
		ValidatorIndex:   uint64(valIndex),
		Height:           uint64(cs.Height),
		Round:            uint64(cs.Round),
		Type:             type_,
		BlockID:          types.BlockID{hash, header},
	}
	err := cs.privValidator.SignVote(cs.state.NTCExtra.ChainID, vote)
	return vote, err
}

func (cs *ConsensusState) signAddVote(type_ byte, hash []byte, header types.PartSetHeader) *types.Vote {

	if cs.privValidator == nil || !cs.Validators.HasAddress(cs.privValidator.GetAddress()) {
		return nil
	}
	vote, err := cs.signVote(type_, hash, header)
	if err == nil {
		if !cs.IsProposer() {
			if cs.ProposerPeerKey != "" {
				v2pMsg := types.EventDataVote2Proposer{vote, cs.ProposerPeerKey}
				types.FireEventVote2Proposer(cs.evsw, v2pMsg)
			} else {
				cs.logger.Warn("sign and vote, Proposer key is nil")
			}
		} else {
			cs.sendInternalMessage(msgInfo{&VoteMessage{vote}, ""})
		}

		cs.logger.Debugf("block is:%+v", vote.BlockID)
		return vote
	} else {
		cs.logger.Warn("Error signing vote", "height", cs.Height, "round", cs.Round, "vote", vote, "error", err)
		return nil
	}
}

func (cs *ConsensusState) sendMaj23SignAggr(voteType byte) {

	var votes []*types.Vote
	var maj23 types.BlockID
	var ok bool

	if voteType == types.VoteTypePrevote {
		votes = cs.Votes.Prevotes(cs.Round).Votes()
		maj23, ok = cs.Votes.Prevotes(cs.Round).TwoThirdsMajority()
	} else if voteType == types.VoteTypePrecommit {
		votes = cs.Votes.Precommits(cs.Round).Votes()
		maj23, ok = cs.Votes.Precommits(cs.Round).TwoThirdsMajority()
	}

	if ok == false {
		cs.logger.Error("Votset does not have +2/3 voting, not send")
		return
	}

	cs.logger.Debugf("vote len is: %v", len(votes))
	numValidators := cs.Validators.Size()
	signBitArray := NewBitArray((uint64)(numValidators))
	var sigs []*tmdcrypto.Signature
	var ss []byte

	for _, vote := range votes {
		if vote != nil && maj23.Equals(vote.BlockID) {
			ss = vote.SignBytes
			signBitArray.SetIndex(vote.ValidatorIndex, true)
			sigs = append(sigs, &(vote.Signature))
		}
	}
	cs.logger.Debugf("send maj block ID: %X", maj23.Hash)

	signature := tmdcrypto.BLSSignatureAggregate(sigs)
	if signature == nil {
		cs.logger.Error("Can not aggregate signature")
		return
	}

	signAggr := types.MakeSignAggr(cs.Height, cs.Round, voteType, numValidators, maj23, cs.Votes.chainID, signBitArray, signature)
	signAggr.SignBytes = ss

	if maj23.IsZero() == true {
		cs.logger.Debugf("The maj23 blockID is zero %+v", maj23)
		return
	}

	signAggr.SetMaj23(maj23)
	cs.logger.Debugf("Generate Maj23SignAggr %#v", signAggr)

	signEvent := types.EventDataSignAggr{SignAggr: signAggr}
	types.FireEventSignAggr(cs.evsw, signEvent)

	cs.sendInternalMessage(msgInfo{&Maj23SignAggrMessage{signAggr}, ""})
}

func CompareHRS(h1 uint64, r1 int, s1 RoundStepType, h2 uint64, r2 int, s2 RoundStepType) int {
	if h1 < h2 {
		return -1
	} else if h1 > h2 {
		return 1
	}

	return 1
}

func (cs *ConsensusState) ValidateTX4(b *types.NCBlock) error {
	var index int

	txs := b.Block.Transactions()
	for _, tx := range txs {
		if neatAbi.IsNeatChainContractAddr(tx.To()) {
			data := tx.Data()
			function, err := neatAbi.FunctionTypeFromId(data[:4])
			if err != nil {
				continue
			}

			if function == neatAbi.WithdrawFromMainChain {

				if index >= len(b.TX3ProofData) {
					return errors.New("tx3 proof data missing")
				}
				tx3ProofData := b.TX3ProofData[index]
				index++

				if err := cs.cch.ValidateTX3ProofData(tx3ProofData); err != nil {
					return err
				}

				if err := cs.cch.ValidateTX4WithInMemTX3ProofData(tx, tx3ProofData); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (cs *ConsensusState) saveBlockToMainChain(block *neatTypes.Block) {

	client := cs.cch.GetClient()
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	bs := []byte{}
	proofData, err := neatTypes.NewSideChainProofData(block)
	if err != nil {
		cs.logger.Error("saveDataToMainChain: failed to create proof data", "block", block, "err", err)
		return
	}
	bs, err = rlp.EncodeToBytes(proofData)
	if err != nil {
		cs.logger.Error("saveDataToMainChain: failed to encode proof data", "proof data", proofData, "err", err)
		return
	}

	cs.logger.Infof("saveDataToMainChain proof data length: %d", len(bs))

	number, err := client.BlockNumber(ctx)
	if err != nil {
		cs.logger.Error("saveDataToMainChain: failed to get BlockNumber at the beginning.", "err", err)
		return
	}

	var prv *ecdsa.PrivateKey
	if prvValidator, ok := cs.privValidator.(*types.PrivValidator); ok {
		prv, err = crypto.ToECDSA(prvValidator.PrivKey.(tmdcrypto.BLSPrivKey).Bytes())
		if err != nil {
			cs.logger.Error("saveDataToMainChain: failed to get PrivateKey", "err", err)
			return
		}
	} else {
		panic("saveDataToMainChain: unexpected privValidator type")
	}
	hash, err := client.SendDataToMainChain(ctx, bs, prv, cs.cch.GetMainChainId())
	if err != nil {
		cs.logger.Error("saveDataToMainChain(rpc) failed", "err", err)
		return
	} else {
		cs.logger.Infof("saveDataToMainChain(rpc) success, hash: %x", hash)
	}

	curNumber := number
	for new(big.Int).Sub(curNumber, number).Int64() < 3 {

		tmpNumber, err := client.BlockNumber(ctx)
		if err != nil {
			cs.logger.Error("saveDataToMainChain: failed to get BlockNumber, abort to wait for 3 blocks", "err", err)
			return
		}

		if tmpNumber.Cmp(curNumber) > 0 {
			_, isPending, err := client.TransactionByHash(ctx, hash)
			if !isPending && err == nil {
				cs.logger.Info("saveDataToMainChain: tx packaged in block in main chain")
				return
			}

			curNumber = tmpNumber
		} else {

			time.Sleep(1 * time.Second)
		}
	}

	cs.logger.Error("saveDataToMainChain: tx not packaged in any block after 3 blocks in main chain")
}

func (cs *ConsensusState) broadcastTX3ProofDataToMainChain(block *neatTypes.Block) {
	client := cs.cch.GetClient()
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	proofData, err := neatTypes.NewTX3ProofData(block)
	if err != nil {
		cs.logger.Error("broadcastTX3ProofDataToMainChain: failed to create proof data", "block", block, "err", err)
		return
	}

	bs, err := rlp.EncodeToBytes(proofData)
	if err != nil {
		cs.logger.Error("broadcastTX3ProofDataToMainChain: failed to encode proof data", "proof data", proofData, "err", err)
		return
	}
	cs.logger.Infof("broadcastTX3ProofDataToMainChain proof data length: %d", len(bs))

	err = client.BroadcastDataToMainChain(ctx, cs.state.NTCExtra.ChainID, bs)
	if err != nil {
		cs.logger.Error("broadcastTX3ProofDataToMainChain(rpc) failed", "err", err)
		return
	}
}
