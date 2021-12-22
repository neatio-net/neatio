package consensus

import (
	"errors"
	"strings"
	"sync"

	"github.com/neatlab/neatio/chain/log"

	"github.com/neatlab/neatio/chain/consensus/neatcon/types"
	. "github.com/neatlib/common-go"
)

type RoundVoteSet struct {
	Prevotes   *types.VoteSet
	Precommits *types.VoteSet
}

type HeightVoteSet struct {
	chainID string
	height  uint64
	valSet  *types.ValidatorSet

	mtx               sync.Mutex
	round             int
	roundVoteSets     map[int]RoundVoteSet
	peerCatchupRounds map[string][]int
	logger            log.Logger
}

func NewHeightVoteSet(chainID string, height uint64, valSet *types.ValidatorSet, logger log.Logger) *HeightVoteSet {
	hvs := &HeightVoteSet{
		chainID: chainID,
		logger:  logger,
	}
	hvs.Reset(height, valSet)
	return hvs
}

func (hvs *HeightVoteSet) Reset(height uint64, valSet *types.ValidatorSet) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	hvs.height = height
	hvs.valSet = valSet
	hvs.roundVoteSets = make(map[int]RoundVoteSet)
	hvs.peerCatchupRounds = make(map[string][]int)

	hvs.addRound(0)
	hvs.round = 0
}

func (hvs *HeightVoteSet) Height() uint64 {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.height
}

func (hvs *HeightVoteSet) Round() int {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.round
}

func (hvs *HeightVoteSet) SetRound(round int) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if hvs.round != 0 && (round < hvs.round+1) {
		PanicSanity("SetRound() must increment hvs.round")
	}
	for r := hvs.round + 1; r <= round; r++ {
		if _, ok := hvs.roundVoteSets[r]; ok {
			continue
		}
		hvs.addRound(r)
	}
	hvs.round = round
}

func (hvs *HeightVoteSet) ResetTop2Round(round int) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if hvs.round < round || round < 1 {
		PanicSanity("hvs.ResetTop2Round() must have enough round to reset")
	}
	for r := round - 1; r <= hvs.round; r++ {
		if _, ok := hvs.roundVoteSets[r]; ok {
			delete(hvs.roundVoteSets, r)
		}
	}

	hvs.addRound(round - 1)
	hvs.addRound(round)
	hvs.round = round
}

func (hvs *HeightVoteSet) addRound(round int) {
	if _, ok := hvs.roundVoteSets[round]; ok {
		PanicSanity("addRound() for an existing round")
	}
	hvs.logger.Debug("addRound(round)", "round", round)
	prevotes := types.NewVoteSet(hvs.chainID, hvs.height, round, types.VoteTypePrevote, hvs.valSet)
	precommits := types.NewVoteSet(hvs.chainID, hvs.height, round, types.VoteTypePrecommit, hvs.valSet)
	hvs.roundVoteSets[round] = RoundVoteSet{
		Prevotes:   prevotes,
		Precommits: precommits,
	}
}

func (hvs *HeightVoteSet) AddVote(vote *types.Vote, peerKey string) (added bool, err error) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if !types.IsVoteTypeValid(vote.Type) {
		return
	}
	voteSet := hvs.getVoteSet(int(vote.Round), vote.Type)
	if voteSet == nil {
		if rndz := hvs.peerCatchupRounds[peerKey]; len(rndz) < 2 {
			hvs.addRound(int(vote.Round))
			voteSet = hvs.getVoteSet(int(vote.Round), vote.Type)
			hvs.peerCatchupRounds[peerKey] = append(rndz, int(vote.Round))
		} else {
			hvs.logger.Warn("Deal with peer giving votes from unwanted rounds")
			return false, errors.New("Deal with peer giving votes from unwanted rounds")
		}
	}
	added, err = voteSet.AddVote(vote)
	return
}

func (hvs *HeightVoteSet) Prevotes(round int) *types.VoteSet {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.getVoteSet(round, types.VoteTypePrevote)
}

func (hvs *HeightVoteSet) Precommits(round int) *types.VoteSet {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.getVoteSet(round, types.VoteTypePrecommit)
}

func (hvs *HeightVoteSet) POLInfo() (polRound int, polBlockID types.BlockID) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	for r := hvs.round; r >= 0; r-- {
		rvs := hvs.getVoteSet(r, types.VoteTypePrevote)
		polBlockID, ok := rvs.TwoThirdsMajority()
		if ok {
			return r, polBlockID
		}
	}
	return -1, types.BlockID{}
}

func (hvs *HeightVoteSet) getVoteSet(round int, type_ byte) *types.VoteSet {
	rvs, ok := hvs.roundVoteSets[round]
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

func (hvs *HeightVoteSet) String() string {
	return hvs.StringIndented("")
}

func (hvs *HeightVoteSet) StringIndented(indent string) string {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	vsStrings := make([]string, 0, (len(hvs.roundVoteSets)+1)*2)
	for round := 0; round <= hvs.round; round++ {
		voteSetString := hvs.roundVoteSets[round].Prevotes.StringShort()
		vsStrings = append(vsStrings, voteSetString)
		voteSetString = hvs.roundVoteSets[round].Precommits.StringShort()
		vsStrings = append(vsStrings, voteSetString)
	}
	for round, roundVoteSet := range hvs.roundVoteSets {
		if round <= hvs.round {
			continue
		}
		voteSetString := roundVoteSet.Prevotes.StringShort()
		vsStrings = append(vsStrings, voteSetString)
		voteSetString = roundVoteSet.Precommits.StringShort()
		vsStrings = append(vsStrings, voteSetString)
	}
	return Fmt(`HeightVoteSet{H:%v R:0~%v
%s  %v
%s}`,
		hvs.height, hvs.round,
		indent, strings.Join(vsStrings, "\n"+indent+"  "),
		indent)
}

func (hvs *HeightVoteSet) SetPeerMaj23(round int, type_ byte, peerID string, blockID types.BlockID) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if !types.IsVoteTypeValid(type_) {
		return
	}
	voteSet := hvs.getVoteSet(round, type_)
	if voteSet == nil {
		return
	}
	voteSet.SetPeerMaj23(peerID, blockID)
}
