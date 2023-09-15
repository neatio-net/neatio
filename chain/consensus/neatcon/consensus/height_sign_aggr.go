package consensus

import (
	"strings"
	"sync"

	. "github.com/nio-net/common"
	"github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/log"
)

type RoundVoteSignAggr struct {
	Prevotes   *types.SignAggr
	Precommits *types.SignAggr
}

type HeightVoteSignAggr struct {
	chainID string
	height  uint64
	valSet  *types.ValidatorSet

	mtx                sync.Mutex
	round              int
	roundVoteSignAggrs map[int]*RoundVoteSignAggr
	logger             log.Logger
}

func NewHeightVoteSignAggr(chainID string, height uint64, valSet *types.ValidatorSet, logger log.Logger) *HeightVoteSignAggr {
	hvs := &HeightVoteSignAggr{
		chainID: chainID,
		logger:  logger,
	}
	hvs.Reset(height, valSet)
	return hvs
}

func (hvs *HeightVoteSignAggr) Reset(height uint64, valSet *types.ValidatorSet) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	hvs.height = height
	hvs.valSet = valSet
	hvs.roundVoteSignAggrs = make(map[int]*RoundVoteSignAggr)

	hvs.addRound(0)
	hvs.round = 0
}

func (hvs *HeightVoteSignAggr) Height() uint64 {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.height
}

func (hvs *HeightVoteSignAggr) Round() int {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.round
}

func (hvs *HeightVoteSignAggr) SetRound(round int) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if hvs.round != 0 && (round < hvs.round+1) {
		PanicSanity("SetRound() must increment hvs.round")
	}
	for r := hvs.round + 1; r <= round; r++ {
		if _, ok := hvs.roundVoteSignAggrs[r]; ok {
			continue
		}
		hvs.addRound(r)
	}
	hvs.round = round
}

func (hvs *HeightVoteSignAggr) ResetTop2Round(round int) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if hvs.round < round || round < 1 {
		PanicSanity("HeightVoteSignAggr.ResetTop2Round() must have enough round to reset")
	}
	for r := round - 1; r <= hvs.round; r++ {
		if _, ok := hvs.roundVoteSignAggrs[r]; ok {
			delete(hvs.roundVoteSignAggrs, r)
		}
	}

	hvs.addRound(round - 1)
	hvs.addRound(round)
	hvs.round = round
}

func (hvs *HeightVoteSignAggr) addRound(round int) {
	if _, ok := hvs.roundVoteSignAggrs[round]; ok {
		PanicSanity("addRound() for an existing round")
	}
	hvs.logger.Debug("addRound(round)", " round:", round)

	hvs.roundVoteSignAggrs[round] = &RoundVoteSignAggr{
		Prevotes:   nil,
		Precommits: nil,
	}
}

func (hvs *HeightVoteSignAggr) AddSignAggr(signAggr *types.SignAggr) (added bool, err error) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if !types.IsVoteTypeValid(signAggr.Type) {
		return
	}
	existing := hvs.getSignAggr(signAggr.Round, signAggr.Type)

	if existing != nil {
		hvs.logger.Warn("Found existing signature aggregation for (height %v round %v type %v)", signAggr.Height, signAggr.Round, signAggr.Type)
		return false, nil

	}

	rvs, ok := hvs.roundVoteSignAggrs[signAggr.Round]

	if !ok {
		hvs.logger.Debugf("round is not existing")
		return false, nil
	}

	if signAggr.Type == types.VoteTypePrevote {
		rvs.Prevotes = signAggr
	} else if signAggr.Type == types.VoteTypePrecommit {
		rvs.Precommits = signAggr
	} else {
		hvs.logger.Warn("Invalid signature aggregation for (height %v round %v type %v)", signAggr.Height, signAggr.Round, signAggr.Type)
		return false, nil
	}

	return true, nil
}

func (hvs *HeightVoteSignAggr) Prevotes(round int) *types.SignAggr {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.getSignAggr(round, types.VoteTypePrevote)
}

func (hvs *HeightVoteSignAggr) Precommits(round int) *types.SignAggr {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.getSignAggr(round, types.VoteTypePrecommit)
}

func (hvs *HeightVoteSignAggr) POLInfo() (polRound int, polBlockID types.BlockID) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	for r := hvs.round; r >= 0; r-- {
		rvs := hvs.getSignAggr(r, types.VoteTypePrevote)
		polBlockID, ok := rvs.TwoThirdsMajority()
		if ok {
			return r, polBlockID
		}
	}
	return -1, types.BlockID{}
}

func (hvs *HeightVoteSignAggr) getSignAggr(round int, type_ byte) *types.SignAggr {
	rvs, ok := hvs.roundVoteSignAggrs[round]
	if !ok {
		return nil
	}
	switch type_ {
	case types.VoteTypePrevote:
		return rvs.Prevotes
	case types.VoteTypePrecommit:
		return rvs.Precommits
	default:
		PanicSanity(Fmt("Unexpected vote type %X", type_))
		return nil
	}
}

func (hvs *HeightVoteSignAggr) String() string {
	return hvs.StringIndented("")
}

func (hvs *HeightVoteSignAggr) StringIndented(indent string) string {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	vsStrings := make([]string, 0, (len(hvs.roundVoteSignAggrs)+1)*2)
	for round := 0; round <= hvs.round; round++ {
		voteSetString := hvs.roundVoteSignAggrs[round].Prevotes.String()
		vsStrings = append(vsStrings, voteSetString)
		voteSetString = hvs.roundVoteSignAggrs[round].Precommits.String()
		vsStrings = append(vsStrings, voteSetString)
	}

	for round, roundVoteSignAggr := range hvs.roundVoteSignAggrs {
		if round <= hvs.round {
			continue
		}
		voteSetString := roundVoteSignAggr.Prevotes.String()
		vsStrings = append(vsStrings, voteSetString)
		voteSetString = roundVoteSignAggr.Precommits.String()
		vsStrings = append(vsStrings, voteSetString)
	}

	return Fmt(`HeightVoteSignAggr{H:%v R:0~%v
%s  %v
%s}`,
		hvs.height, hvs.round,
		indent, strings.Join(vsStrings, "\n"+indent+"  "),
		indent)
}
