package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"sort"

	"github.com/neatlab/neatio/chain/trie"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/rlp"
)

func (self *StateDB) GetTotalRewardBalance(addr common.Address) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.RewardBalance()
	}
	return common.Big0
}

func (self *StateDB) AddRewardBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {

		stateObject.AddRewardBalance(amount)
	}
}

func (self *StateDB) SubRewardBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {

		stateObject.SubRewardBalance(amount)
	}
}

func (self *StateDB) GetTotalAvailableRewardBalance(addr common.Address) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.AvailableRewardBalance()
	}
	return common.Big0
}

func (self *StateDB) AddAvailableRewardBalance(addr common.Address, amount *big.Int) {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		stateObject.AddAvailableRewardBalance(amount)
	}
}

func (self *StateDB) SubAvailableRewardBalance(addr common.Address, amount *big.Int) {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		stateObject.SubAvailableRewardBalance(amount)
	}
}

func (self *StateDB) GetDelegateRewardAddress(addr common.Address) map[common.Address]struct{} {
	var deleAddr map[common.Address]struct{}
	reward := Reward{}

	so := self.getStateObject(addr)
	if so == nil {
		return deleAddr
	}

	it := trie.NewIterator(so.getRewardTrie(self.db).NodeIterator(nil))
	for it.Next() {
		var key common.Address
		rlp.DecodeBytes(self.trie.GetKey(it.Key), &key)
		deleAddr[key] = struct{}{}
	}

	if len(so.dirtyReward) > len(deleAddr) {
		reward = so.dirtyReward
		for key := range reward {
			deleAddr[key] = struct{}{}
		}
	}

	if len(so.originReward) > len(deleAddr) {
		reward = so.originReward
		for key := range reward {
			deleAddr[key] = struct{}{}
		}
	}

	return deleAddr
}

func (self *StateDB) GetRewardBalanceByDelegateAddress(addr common.Address, deleAddress common.Address) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		rewardBalance := stateObject.GetDelegateRewardBalance(self.db, deleAddress)
		if rewardBalance == nil {
			return common.Big0
		} else {
			return rewardBalance
		}
	}
	return common.Big0
}

func (self *StateDB) AddRewardBalanceByDelegateAddress(addr common.Address, deleAddress common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {

		rewardBalance := stateObject.GetDelegateRewardBalance(self.db, deleAddress)
		var dirtyRewardBalance *big.Int
		if rewardBalance == nil {
			dirtyRewardBalance = amount
		} else {
			dirtyRewardBalance = new(big.Int).Add(rewardBalance, amount)
		}
		stateObject.SetDelegateRewardBalance(self.db, deleAddress, dirtyRewardBalance)

		stateObject.AddRewardBalance(amount)
	}
}

func (self *StateDB) SubRewardBalanceByDelegateAddress(addr common.Address, deleAddress common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {

		rewardBalance := stateObject.GetDelegateRewardBalance(self.db, deleAddress)
		var dirtyRewardBalance *big.Int
		if rewardBalance == nil {
			panic("you can't subtract the amount from nil balance, check the code, this should not happen")
		} else {
			dirtyRewardBalance = new(big.Int).Sub(rewardBalance, amount)
		}
		stateObject.SetDelegateRewardBalance(self.db, deleAddress, dirtyRewardBalance)

		stateObject.SubRewardBalance(amount)
	}
}

func (db *StateDB) ForEachReward(addr common.Address, cb func(key common.Address, rewardBalance *big.Int) bool) {
	so := db.getStateObject(addr)
	if so == nil {
		return
	}
	it := trie.NewIterator(so.getRewardTrie(db.db).NodeIterator(nil))
	for it.Next() {
		var key common.Address
		rlp.DecodeBytes(db.trie.GetKey(it.Key), &key)
		if value, dirty := so.dirtyReward[key]; dirty {
			cb(key, value)
			continue
		}
		var value big.Int
		rlp.DecodeBytes(it.Value, &value)
		cb(key, &value)
	}
}

func (self *StateDB) MarkAddressReward(addr common.Address) {
	if _, exist := self.GetRewardSet()[addr]; !exist {
		self.rewardSet[addr] = struct{}{}
		self.rewardSetDirty = true
	}
}

func (self *StateDB) GetRewardSet() RewardSet {
	if len(self.rewardSet) != 0 {
		return self.rewardSet
	}

	enc, err := self.trie.TryGet(rewardSetKey)
	if err != nil {
		self.setError(err)
		return nil
	}
	var value RewardSet
	if len(enc) > 0 {
		err := rlp.DecodeBytes(enc, &value)
		if err != nil {
			self.setError(err)
		}
		self.rewardSet = value
	}
	return value
}

func (self *StateDB) commitRewardSet() {
	data, err := rlp.EncodeToBytes(self.rewardSet)
	if err != nil {
		panic(fmt.Errorf("can't encode reward set : %v", err))
	}
	self.setError(self.trie.TryUpdate(rewardSetKey, data))
}

func (self *StateDB) ClearRewardSetByAddress(addr common.Address) {
	delete(self.rewardSet, addr)
	self.rewardSetDirty = true
}

var rewardSetKey = []byte("RewardSet")

type RewardSet map[common.Address]struct{}

func (set RewardSet) EncodeRLP(w io.Writer) error {
	var list []common.Address
	for addr := range set {
		list = append(list, addr)
	}
	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].Bytes(), list[j].Bytes()) == 1
	})
	return rlp.Encode(w, list)
}

func (set *RewardSet) DecodeRLP(s *rlp.Stream) error {
	var list []common.Address
	if err := s.Decode(&list); err != nil {
		return err
	}
	rewardSet := make(RewardSet, len(list))
	for _, addr := range list {
		rewardSet[addr] = struct{}{}
	}
	*set = rewardSet
	return nil
}

func (self *StateDB) SetSideChainRewardPerBlock(rewardPerBlock *big.Int) {
	self.sideChainRewardPerBlock = rewardPerBlock
	self.sideChainRewardPerBlockDirty = true
}

func (self *StateDB) GetSideChainRewardPerBlock() *big.Int {
	if self.sideChainRewardPerBlock != nil {
		return self.sideChainRewardPerBlock
	}

	enc, err := self.trie.TryGet(sideChainRewardPerBlockKey)
	if err != nil {
		self.setError(err)
		return nil
	}
	value := new(big.Int)
	if len(enc) > 0 {
		err := rlp.DecodeBytes(enc, value)
		if err != nil {
			self.setError(err)
		}
		self.sideChainRewardPerBlock = value
	}
	return value
}

func (self *StateDB) commitSideChainRewardPerBlock() {
	data, err := rlp.EncodeToBytes(self.sideChainRewardPerBlock)
	if err != nil {
		panic(fmt.Errorf("can't encode side chain reward per block : %v", err))
	}
	self.setError(self.trie.TryUpdate(sideChainRewardPerBlockKey, data))
}

var sideChainRewardPerBlockKey = []byte("RewardPerBlock")
