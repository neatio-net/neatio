package types

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
	"strings"

	cmn "github.com/neatio-network/common-go"
	"github.com/neatio-network/crypto-go"
	"github.com/neatio-network/merkle-go"
	"github.com/neatio-network/neatio/chain/log"
	"github.com/neatio-network/neatio/utilities/common"
)

type ValidatorSet struct {
	Validators []*Validator `json:"validators"`

	totalVotingPower *big.Int
}

func NewValidatorSet(vals []*Validator) *ValidatorSet {
	validators := make([]*Validator, len(vals))
	for i, val := range vals {
		validators[i] = val.Copy()
	}
	sort.Sort(ValidatorsByAddress(validators))
	vs := &ValidatorSet{
		Validators: validators,
	}

	return vs
}

func (valSet *ValidatorSet) Copy() *ValidatorSet {
	validators := make([]*Validator, len(valSet.Validators))
	for i, val := range valSet.Validators {
		validators[i] = val.Copy()
	}
	return &ValidatorSet{
		Validators:       validators,
		totalVotingPower: valSet.totalVotingPower,
	}
}

func (valSet *ValidatorSet) AggrPubKey(bitMap *cmn.BitArray) crypto.PubKey {
	if bitMap == nil {
		return nil
	}
	if (int)(bitMap.Size()) != len(valSet.Validators) {
		return nil
	}
	validators := valSet.Validators
	var pks []*crypto.PubKey
	for i := (uint64)(0); i < bitMap.Size(); i++ {
		if bitMap.GetIndex(i) {
			pks = append(pks, &(validators[i].PubKey))
		}
	}
	return crypto.BLSPubKeyAggregate(pks)
}

func (valSet *ValidatorSet) GetAggrPubKeyAndAddress(bitMap *cmn.BitArray) (*ConsensusAggr, error) {
	if bitMap == nil {
		return nil, fmt.Errorf("invalid bitmap(nil)")
	}
	if (int)(bitMap.Size()) != valSet.Size() {
		return nil, fmt.Errorf("size is not equal, validators size:%v, bitmap size:%v", valSet.Size(), bitMap.Size())
	}
	validators := valSet.Validators

	var pks = make([]string, bitMap.Size())
	var addr = make([]common.Address, bitMap.Size())
	for i := (uint64)(0); i < bitMap.Size(); i++ {
		if bitMap.GetIndex(i) {
			pks[i] = (validators[i].PubKey).KeyString()
			addr[i] = common.BytesToAddress(validators[i].Address)
		}
	}
	var aggr = &ConsensusAggr{}
	aggr.PublicKeys = pks
	aggr.Addresses = addr
	return aggr, nil
}

func (valSet *ValidatorSet) TalliedVotingPower(bitMap *cmn.BitArray) (*big.Int, *big.Int, *big.Int, error) {
	if bitMap == nil {
		return big.NewInt(0), big.NewInt(0), big.NewInt(0), fmt.Errorf("invalid bitmap(nil)")
	}
	validators := valSet.Validators
	if validators == nil {
		return big.NewInt(0), big.NewInt(0), big.NewInt(0), fmt.Errorf("invalid validators(nil)")
	}
	if valSet.Size() != (int)(bitMap.Size()) {
		return big.NewInt(0), big.NewInt(0), big.NewInt(0), fmt.Errorf("size is not equal, validators size:%v, bitmap size:%v", valSet.Size(), bitMap.Size())
	}
	powerSum := big.NewInt(0)
	votesSum := big.NewInt(0)
	totalVotes := big.NewInt(0)
	for i := (uint64)(0); i < bitMap.Size(); i++ {
		if bitMap.GetIndex(i) {
			powerSum.Add(powerSum, common.Big1)
			votesSum.Add(votesSum, validators[i].VotingPower)
		}
		totalVotes.Add(totalVotes, validators[i].VotingPower)
	}
	return powerSum, votesSum, totalVotes, nil
}

func (valSet *ValidatorSet) Equals(other *ValidatorSet) bool {

	if len(valSet.Validators) != len(other.Validators) {
		return false
	}

	for _, v := range other.Validators {

		_, val := valSet.GetByAddress(v.Address)
		if val == nil || !val.Equals(v) {
			return false
		}
	}

	return true
}

func (valSet *ValidatorSet) HasAddress(address []byte) bool {

	for i := 0; i < len(valSet.Validators); i++ {
		if bytes.Compare(address, valSet.Validators[i].Address) == 0 {
			return true
		}
	}
	return false
}

func (valSet *ValidatorSet) GetByAddress(address []byte) (index int, val *Validator) {

	idx := -1
	for i := 0; i < len(valSet.Validators); i++ {
		if bytes.Compare(address, valSet.Validators[i].Address) == 0 {
			idx = i
			break
		}
	}

	if idx != -1 {
		return idx, valSet.Validators[idx].Copy()
	} else {
		return 0, nil
	}
}

func (valSet *ValidatorSet) GetByIndex(index int) (address []byte, val *Validator) {
	val = valSet.Validators[index]
	return val.Address, val.Copy()
}

func (valSet *ValidatorSet) Size() int {
	return len(valSet.Validators)
}

func (valSet *ValidatorSet) TotalVotingPower() *big.Int {
	return big.NewInt(int64(valSet.Size()))
}

func (valSet *ValidatorSet) Hash() []byte {
	if len(valSet.Validators) == 0 {
		return nil
	}
	hashables := make([]merkle.Hashable, len(valSet.Validators))
	for i, val := range valSet.Validators {
		hashables[i] = val
	}
	return merkle.SimpleHashFromHashables(hashables)
}

func (valSet *ValidatorSet) Add(val *Validator) (added bool) {
	val = val.Copy()

	idx := -1
	for i := 0; i < len(valSet.Validators); i++ {
		if bytes.Compare(val.Address, valSet.Validators[i].Address) == 0 {
			idx = i
			break
		}
	}

	if idx == -1 {
		valSet.Validators = append(valSet.Validators, val)

		valSet.totalVotingPower = nil
		return true
	} else {
		return false
	}
}

func (valSet *ValidatorSet) Update(val *Validator) (updated bool) {
	index, sameVal := valSet.GetByAddress(val.Address)
	if sameVal == nil {
		return false
	} else {
		valSet.Validators[index] = val.Copy()
		valSet.totalVotingPower = nil
		return true
	}
}

func (valSet *ValidatorSet) Remove(address []byte) (val *Validator, removed bool) {
	idx := -1
	for i := 0; i < len(valSet.Validators); i++ {
		if bytes.Compare(address, valSet.Validators[i].Address) == 0 {
			idx = i
			break
		}
	}

	if idx == -1 {
		return nil, false
	} else {
		removedVal := valSet.Validators[idx]
		newValidators := valSet.Validators[:idx]
		if idx+1 < len(valSet.Validators) {
			newValidators = append(newValidators, valSet.Validators[idx+1:]...)
		}
		valSet.Validators = newValidators
		valSet.totalVotingPower = nil
		return removedVal, true
	}
}

func (valSet *ValidatorSet) Iterate(fn func(index int, val *Validator) bool) {
	for i, val := range valSet.Validators {
		stop := fn(i, val.Copy())
		if stop {
			break
		}
	}
}

func (valSet *ValidatorSet) VerifyCommit(chainID string, height uint64, commit *Commit) error {

	log.Debugf("avoid valSet and commit.Precommits size check for validatorset change")
	if commit == nil {
		return fmt.Errorf("invalid commit(nil)")
	}
	if (uint64)(valSet.Size()) != commit.BitArray.Size() {
		return fmt.Errorf("invalid commit -- wrong set size: %v vs %v", valSet.Size(), commit.BitArray.Size())
	}
	if height != commit.Height {
		return fmt.Errorf("invalid commit -- wrong height: %v vs %v", height, commit.Height)
	}

	pubKey := valSet.AggrPubKey(commit.BitArray)
	vote := &Vote{

		BlockID: commit.BlockID,
		Height:  commit.Height,
		Round:   (uint64)(commit.Round),
		Type:    commit.Type(),
	}
	if !pubKey.VerifyBytes(SignBytes(chainID, vote), commit.SignAggr) {
		return fmt.Errorf("invalid commit -- wrong Signature:%v or BitArray:%v", commit.SignAggr, commit.BitArray)
	}

	_, votesSum, totalVotes, err := valSet.TalliedVotingPower(commit.BitArray)
	log.Debugf("VerifyCommit talliedVotes %v, totalVotes %v", votesSum, totalVotes)
	if err != nil {
		return err
	}

	quorum := Loose23MajorThreshold(totalVotes, commit.Round)
	log.Debugf("Loose 2/3 major threshold  quorum %v", quorum)
	if votesSum.Cmp(quorum) >= 0 {
		return nil
	} else {
		return fmt.Errorf("invalid commit -- insufficient voting power: got %v, needed %v",
			votesSum, quorum)
	}
}

func (valSet *ValidatorSet) VerifyCommitAny(chainID string, blockID BlockID, height int, commit *Commit) error {
	panic("Not yet implemented")
}

func (valSet *ValidatorSet) String() string {
	return valSet.StringIndented("")
}

func (valSet *ValidatorSet) StringIndented(indent string) string {
	if valSet == nil {
		return "nil-ValidatorSet"
	}
	valStrings := []string{}
	valSet.Iterate(func(index int, val *Validator) bool {
		valStrings = append(valStrings, val.String())
		return false
	})
	return fmt.Sprintf(`ValidatorSet{
%s  Validators:
%s    %v
%s}`,
		indent,
		indent, strings.Join(valStrings, "\n"+indent+"    "),
		indent)

}

type ValidatorsByAddress []*Validator

func (vs ValidatorsByAddress) Len() int {
	return len(vs)
}

func (vs ValidatorsByAddress) Less(i, j int) bool {
	return bytes.Compare(vs[i].Address, vs[j].Address) == -1
}

func (vs ValidatorsByAddress) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
