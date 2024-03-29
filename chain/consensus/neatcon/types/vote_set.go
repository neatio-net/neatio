package types

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"math/big"

	. "github.com/neatio-net/common-go"
)

/*
	VoteSet helps collect signatures from validators at each height+round for a
	predefined vote type.

	We need VoteSet to be able to keep track of conflicting votes when validators
	double-sign.  Yet, we can't keep track of *all* the votes seen, as that could
	be a DoS attack vector.

	There are two storage areas for votes.
	1. voteSet.votes
	2. voteSet.votesByBlock

	`.votes` is the "canonical" list of votes.  It always has at least one vote,
	if a vote from a validator had been seen at all.  Usually it keeps track of
	the first vote seen, but when a 2/3 majority is found, votes for that get
	priority and are copied over from `.votesByBlock`.

	`.votesByBlock` keeps track of a list of votes for a particular block.  There
	are two ways a &blockVotes{} gets created in `.votesByBlock`.
	1. the first vote seen by a validator was for the particular block.
	2. a peer claims to have seen 2/3 majority for the particular block.

	Since the first vote from a validator will always get added in `.votesByBlock`
	, all votes in `.votes` will have a corresponding entry in `.votesByBlock`.

	When a &blockVotes{} in `.votesByBlock` reaches a 2/3 majority quorum, its
	votes are copied into `.votes`.

	All this is memory bounded because conflicting votes only get added if a peer
	told us to track that block, each peer only gets to tell us 1 such block, and,
	there's only a limited number of peers.

	NOTE: Assumes that the sum total of voting power does not exceed MaxUInt64.
*/

const LooseRound = 30
const HalfLooseRound = 15

//max { [(2*LooseRound - round)*totalVotingPower + 3*LooseRound]/(3*LooseRound), totalVotingPower/2 + 1 }
func Loose23MajorThreshold(totalVotingPower *big.Int, round int) *big.Int {

	//quorum := big.NewInt(0)
	//quorum.Add(totalVotingPower, big.NewInt(3))
	//quorum.Div(quorum, big.NewInt(3))

	//if round >= LooseRound {
	//	return quorum
	//}

	if round >= HalfLooseRound {
		round = HalfLooseRound
	}

	quorum1 := big.NewInt(0)
	quorum1.Mul(totalVotingPower, big.NewInt(int64(2*LooseRound-round)))
	quorum1.Add(quorum1, big.NewInt(3*LooseRound))
	quorum1.Div(quorum1, big.NewInt(3*LooseRound))

	//if quorum.Cmp(quorum1) > 0 {
	//	return quorum
	//} else {
	//	return quorum1
	//}
	return quorum1
}

type VoteSet struct {
	chainID string
	height  uint64
	round   int
	type_   byte

	mtx           sync.Mutex
	valSet        *ValidatorSet
	votesBitArray *BitArray
	votes         []*Vote                // Primary votes to share
	sum           *big.Int               // Sum of voting power for seen votes, discounting conflicts
	maj23         *BlockID               // First 2/3 majority seen
	votesByBlock  map[string]*blockVotes // string(blockHash|blockParts) -> blockVotes
	peerMaj23s    map[string]BlockID     // Maj23 for each peer
}

// Constructs a new VoteSet struct used to accumulate votes for given height/round.
func NewVoteSet(chainID string, height uint64, round int, type_ byte, valSet *ValidatorSet) *VoteSet {
	if height == 0 {
		PanicSanity("Cannot make VoteSet for height == 0, doesn't make sense.")
	}
	return &VoteSet{
		chainID:       chainID,
		height:        height,
		round:         round,
		type_:         type_,
		valSet:        valSet,
		votesBitArray: NewBitArray(uint64(valSet.Size())),
		votes:         make([]*Vote, valSet.Size()),
		sum:           big.NewInt(0),
		maj23:         nil,
		votesByBlock:  make(map[string]*blockVotes, valSet.Size()),
		peerMaj23s:    make(map[string]BlockID),
	}
}

func (voteSet *VoteSet) ChainID() string {
	return voteSet.chainID
}

func (voteSet *VoteSet) Height() uint64 {
	if voteSet == nil {
		return 0
	} else {
		return voteSet.height
	}
}

func (voteSet *VoteSet) Round() int {
	if voteSet == nil {
		return -1
	} else {
		return voteSet.round
	}
}

func (voteSet *VoteSet) Type() byte {
	if voteSet == nil {
		return 0x00
	} else {
		return voteSet.type_
	}
}

func (voteSet *VoteSet) Votes() []*Vote {
	return voteSet.votes
}

func (voteSet *VoteSet) Size() int {
	if voteSet == nil {
		return 0
	} else {
		return voteSet.valSet.Size()
	}
}

// Returns added=true if vote is valid and new.
// Otherwise returns err=ErrVote[
//		UnexpectedStep | InvalidIndex | InvalidAddress |
//		InvalidSignature | InvalidBlockHash | ConflictingVotes ]
// Duplicate votes return added=false, err=nil.
// Conflicting votes return added=*, err=ErrVoteConflictingVotes.
// NOTE: vote should not be mutated after adding.
// NOTE: VoteSet must not be nil
func (voteSet *VoteSet) AddVote(vote *Vote) (added bool, err error) {
	if voteSet == nil {
		PanicSanity("AddVote() on nil VoteSet")
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()

	return voteSet.addVote(vote)
}

// NOTE: Validates as much as possible before attempting to verify the signature.
func (voteSet *VoteSet) addVote(vote *Vote) (added bool, err error) {
	valIndex := vote.ValidatorIndex
	valAddr := vote.ValidatorAddress
	blockKey := vote.BlockID.Key()

	// Ensure that validator index was set
	if valIndex < 0 || len(valAddr) == 0 {
		panic("Validator index or address was not set in vote.")
	}

	// Make sure the step matches.
	if (vote.Height != voteSet.height) ||
		(int(vote.Round) != voteSet.round) ||
		(vote.Type != voteSet.type_) {
		return false, ErrVoteUnexpectedStep
	}

	// Ensure that signer is a validator.
	lookupAddr, val := voteSet.valSet.GetByIndex(int(valIndex))
	if val == nil {
		return false, ErrVoteInvalidValidatorIndex
	}

	// Ensure that the signer has the right address
	if !bytes.Equal(valAddr, lookupAddr) {
		return false, ErrVoteInvalidValidatorAddress
	}

	// If we already know of this vote, return false.
	if existing, ok := voteSet.getVote(int(valIndex), blockKey); ok {
		if existing.Signature.Equals(vote.Signature) {
			return false, nil // duplicate
		} else {
			return false, ErrVoteInvalidSignature // NOTE: assumes deterministic signatures
		}
	}

	// Check signature.
	if !val.PubKey.VerifyBytes(SignBytes(voteSet.chainID, vote), vote.Signature) {
		// Bad signature.
		return false, ErrVoteInvalidSignature
	}

	// Add vote and get conflicting vote if any
	//added, conflicting := voteSet.addVerifiedVote(vote, blockKey, common.Big1)
	added, conflicting := voteSet.addVerifiedVote(vote, blockKey, val.VotingPower)
	if conflicting != nil {
		return added, &ErrVoteConflictingVotes{
			VoteA: conflicting,
			VoteB: vote,
		}
	} else {
		if !added {
			PanicSanity("Expected to add non-conflicting vote")
		}
		return added, nil
	}

}

// Returns (vote, true) if vote exists for valIndex and blockKey
func (voteSet *VoteSet) getVote(valIndex int, blockKey string) (vote *Vote, ok bool) {
	if existing := voteSet.votes[valIndex]; existing != nil && existing.BlockID.Key() == blockKey {
		return existing, true
	}
	if existing := voteSet.votesByBlock[blockKey].getByIndex(valIndex); existing != nil {
		return existing, true
	}
	return nil, false
}

// Assumes signature is valid.
// If conflicting vote exists, returns it.
func (voteSet *VoteSet) addVerifiedVote(vote *Vote, blockKey string, votingPower *big.Int) (added bool, conflicting *Vote) {
	valIndex := vote.ValidatorIndex

	// Already exists in voteSet.votes?
	if existing := voteSet.votes[valIndex]; existing != nil {
		if existing.BlockID.Equals(vote.BlockID) {
			PanicSanity("addVerifiedVote does not expect duplicate votes")
		} else {
			conflicting = existing
		}
		// Replace vote if blockKey matches voteSet.maj23.
		if voteSet.maj23 != nil && voteSet.maj23.Key() == blockKey {
			voteSet.votes[valIndex] = vote
			voteSet.votesBitArray.SetIndex(valIndex, true)
		}
		// Otherwise don't add it to voteSet.votes
	} else {
		// Add to voteSet.votes and incr .sum
		voteSet.votes[valIndex] = vote
		voteSet.votesBitArray.SetIndex(valIndex, true)
		voteSet.sum.Add(voteSet.sum, votingPower)
	}

	votesByBlock, ok := voteSet.votesByBlock[blockKey]
	if ok {
		if conflicting != nil && !votesByBlock.peerMaj23 {
			// There's a conflict and no peer claims that this block is special.
			return false, conflicting
		}
		// We'll add the vote in a bit.
	} else {
		// .votesByBlock doesn't exist...
		if conflicting != nil {
			// ... and there's a conflicting vote.
			// We're not even tracking this blockKey, so just forget it.
			return false, conflicting
		} else {
			// ... and there's no conflicting vote.
			// Start tracking this blockKey
			votesByBlock = newBlockVotes(false, voteSet.valSet.Size())
			voteSet.votesByBlock[blockKey] = votesByBlock
			// We'll add the vote in a bit.
		}
	}

	// Before adding to votesByBlock, see if we'll exceed quorum
	origSum := new(big.Int).Set(votesByBlock.sum)

	_, _, totalVotes, err := voteSet.valSet.TalliedVotingPower(votesByBlock.bitArray)

	if err != nil {
		return false, conflicting
	}

	/*
		twoThird := new(big.Int).Mul(voteSet.valSet.TotalVotingPower(), big.NewInt(2))
		twoThird.Div(twoThird, big.NewInt(3))
		quorum := new(big.Int).Add(twoThird, big.NewInt(1))
	*/
	//quorum := Loose23MajorThreshold(voteSet.valSet.TotalVotingPower(), int(vote.Round))
	quorum := Loose23MajorThreshold(totalVotes, int(vote.Round))

	// Add vote to votesByBlock
	votesByBlock.addVerifiedVote(vote, votingPower)

	//voteSum := origSum.Add(origSum, voteSet.valSet.Validators[vote.ValidatorIndex].VotingPower)

	//fmt.Printf("add verified vote, origsum: %v, votesum: %v, totalvotes: %v, quorum: %v\n", origSum, votesByBlock.sum, totalVotes, quorum)

	// If we just crossed the quorum threshold and have 2/3 majority...
	// if origSum < quorum && quorum <= votesByBlock.sum {
	//if origSum.Cmp(quorum) == -1 && quorum.Cmp(votesByBlock.sum) <= 0 {

	if origSum.Cmp(quorum) == -1 && quorum.Cmp(votesByBlock.sum) <= 0 {
		// Only consider the first quorum reached
		if voteSet.maj23 == nil {
			maj23BlockID := vote.BlockID
			voteSet.maj23 = &maj23BlockID
			// And also copy votes over to voteSet.votes
			for i, vote := range votesByBlock.votes {
				if vote != nil {
					voteSet.votes[i] = vote
				}
			}
		}
	}

	return true, conflicting
}

// If a peer claims that it has 2/3 majority for given blockKey, call this.
// NOTE: if there are too many peers, or too much peer churn,
// this can cause memory issues.
// TODO: implement ability to remove peers too
// NOTE: VoteSet must not be nil
func (voteSet *VoteSet) SetPeerMaj23(peerID string, blockID BlockID) {
	if voteSet == nil {
		PanicSanity("SetPeerMaj23() on nil VoteSet")
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()

	blockKey := blockID.Key()

	// Make sure peer hasn't already told us something.
	if existing, ok := voteSet.peerMaj23s[peerID]; ok {
		if existing.Equals(blockID) {
			return // Nothing to do
		} else {
			return // TODO bad peer!
		}
	}
	voteSet.peerMaj23s[peerID] = blockID

	// Create .votesByBlock entry if needed.
	votesByBlock, ok := voteSet.votesByBlock[blockKey]
	if ok {
		if votesByBlock.peerMaj23 {
			return // Nothing to do
		} else {
			votesByBlock.peerMaj23 = true
			// No need to copy votes, already there.
		}
	} else {
		votesByBlock = newBlockVotes(true, voteSet.valSet.Size())
		voteSet.votesByBlock[blockKey] = votesByBlock
		// No need to copy votes, no votes to copy over.
	}
}

func (voteSet *VoteSet) BitArray() *BitArray {
	if voteSet == nil {
		return nil
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	return voteSet.votesBitArray.Copy()
}

func (voteSet *VoteSet) BitArrayByBlockID(blockID BlockID) *BitArray {
	if voteSet == nil {
		return nil
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	votesByBlock, ok := voteSet.votesByBlock[blockID.Key()]
	if ok {
		return votesByBlock.bitArray.Copy()
	}
	return nil
}

// NOTE: if validator has conflicting votes, returns "canonical" vote
func (voteSet *VoteSet) GetByIndex(valIndex int) *Vote {
	if voteSet == nil {
		return nil
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	return voteSet.votes[valIndex]
}

func (voteSet *VoteSet) GetByAddress(address []byte) *Vote {
	if voteSet == nil {
		return nil
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	valIndex, val := voteSet.valSet.GetByAddress(address)
	if val == nil {
		PanicSanity("GetByAddress(address) returned nil")
	}
	return voteSet.votes[valIndex]
}

func (voteSet *VoteSet) HasTwoThirdsMajority() bool {
	if voteSet == nil {
		return false
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	return voteSet.maj23 != nil
}

func (voteSet *VoteSet) IsCommit() bool {
	if voteSet == nil {
		return false
	}
	if voteSet.type_ != VoteTypePrecommit {
		return false
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	return voteSet.maj23 != nil
}

//func (voteSet *VoteSet) HasTwoThirdsAny() bool {
//	if voteSet == nil {
//		return false
//	}
//	voteSet.mtx.Lock()
//	defer voteSet.mtx.Unlock()
//
//	/*
//		twoThird := new(big.Int).Mul(voteSet.valSet.TotalVotingPower(), big.NewInt(2))
//		twoThird.Div(twoThird, big.NewInt(3))
//	*/
//	twoThirdPlus1 := Loose23MajorThreshold(voteSet.valSet.TotalVotingPower(), voteSet.round)
//	twoThird := twoThirdPlus1.Sub(twoThirdPlus1, big.NewInt(1))
//
//	return voteSet.sum.Cmp(twoThird) == 1
//}

func (voteSet *VoteSet) HasAll() bool {
	return voteSet.sum == voteSet.valSet.TotalVotingPower()
}

// Returns either a blockhash (or nil) that received +2/3 majority.
// If there exists no such majority, returns (nil, PartSetHeader{}, false).
func (voteSet *VoteSet) TwoThirdsMajority() (blockID BlockID, ok bool) {
	if voteSet == nil {
		return BlockID{}, false
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	if voteSet.maj23 != nil {
		return *voteSet.maj23, true
	} else {
		return BlockID{}, false
	}
}

func (voteSet *VoteSet) String() string {
	if voteSet == nil {
		return "nil-VoteSet"
	}
	return voteSet.StringIndented("")
}

func (voteSet *VoteSet) StringIndented(indent string) string {
	voteStrings := make([]string, len(voteSet.votes))
	for i, vote := range voteSet.votes {
		if vote == nil {
			voteStrings[i] = "nil-Vote"
		} else {
			voteStrings[i] = vote.String()
		}
	}
	return fmt.Sprintf(`VoteSet{
%s  H:%v R:%v T:%v
%s  %v
%s  %v
%s  %v
%s}`,
		indent, voteSet.height, voteSet.round, voteSet.type_,
		indent, strings.Join(voteStrings, "\n"+indent+"  "),
		indent, voteSet.votesBitArray,
		indent, voteSet.peerMaj23s,
		indent)
}

func (voteSet *VoteSet) StringShort() string {
	if voteSet == nil {
		return "nil-VoteSet"
	}
	voteSet.mtx.Lock()
	defer voteSet.mtx.Unlock()
	return fmt.Sprintf(`VoteSet{H:%v R:%v T:%v +2/3:%v %v %v}`,
		voteSet.height, voteSet.round, voteSet.type_, voteSet.maj23, voteSet.votesBitArray, voteSet.peerMaj23s)
}

//--------------------------------------------------------------------------------

/*
	Votes for a particular block
	There are two ways a *blockVotes gets created for a blockKey.
	1. first (non-conflicting) vote of a validator w/ blockKey (peerMaj23=false)
	2. A peer claims to have a 2/3 majority w/ blockKey (peerMaj23=true)
*/
type blockVotes struct {
	peerMaj23 bool      // peer claims to have maj23
	bitArray  *BitArray // valIndex -> hasVote?
	votes     []*Vote   // valIndex -> *Vote
	sum       *big.Int  // vote sum
}

func newBlockVotes(peerMaj23 bool, numValidators int) *blockVotes {
	return &blockVotes{
		peerMaj23: peerMaj23,
		bitArray:  NewBitArray(uint64(numValidators)),
		votes:     make([]*Vote, numValidators),
		sum:       big.NewInt(0),
	}
}

func (vs *blockVotes) addVerifiedVote(vote *Vote, votingPower *big.Int) {
	valIndex := vote.ValidatorIndex
	if existing := vs.votes[valIndex]; existing == nil {
		vs.bitArray.SetIndex(valIndex, true)
		vs.votes[valIndex] = vote
		vs.sum.Add(vs.sum, votingPower)
	}
}

func (vs *blockVotes) getByIndex(index int) *Vote {
	if vs == nil {
		return nil
	}
	return vs.votes[index]
}

//--------------------------------------------------------------------------------

// Common interface between *consensus.VoteSet and types.Commit
type VoteSetReader interface {
	Height() uint64
	Round() int
	Type() byte
	Size() int
	BitArray() *BitArray
	GetByIndex(int) *Vote
	IsCommit() bool
}
