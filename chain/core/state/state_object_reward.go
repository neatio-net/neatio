package state

import (
	"fmt"
	"math/big"

	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/rlp"
)

type Reward map[common.Address]*big.Int

func (p Reward) String() (str string) {
	for key, value := range p {
		str += fmt.Sprintf("Address %v : %v\n", key.String(), value)
	}
	return
}

func (p Reward) Copy() Reward {
	cpy := make(Reward)
	for key, value := range p {

		cpy[key] = value
	}
	return cpy
}

func (c *stateObject) AddRewardBalance(amount *big.Int) {

	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}
		return
	}
	c.SetRewardBalance(new(big.Int).Add(c.RewardBalance(), amount))
}

func (c *stateObject) SubRewardBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetRewardBalance(new(big.Int).Sub(c.RewardBalance(), amount))
}

func (self *stateObject) SetRewardBalance(amount *big.Int) {
	if amount.Sign() < 0 {
		log.Infof("!!!amount is negative, not support yet, make it 0 by force")
		amount = big.NewInt(0)
	}

	self.db.journal = append(self.db.journal, rewardBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.RewardBalance),
	})
	self.setRewardBalance(amount)
}

func (self *stateObject) setRewardBalance(amount *big.Int) {
	self.data.RewardBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) RewardBalance() *big.Int {
	return self.data.RewardBalance
}

func (c *stateObject) getRewardTrie(db Database) Trie {
	if c.rewardTrie == nil {
		var err error
		c.rewardTrie, err = db.OpenRewardTrie(c.addrHash, c.data.RewardRoot)
		if err != nil {
			c.rewardTrie, _ = db.OpenRewardTrie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create reward trie: %v", err))
		}
	}
	return c.rewardTrie
}

func (self *stateObject) GetDelegateRewardBalance(db Database, key common.Address) *big.Int {

	value, dirty := self.dirtyReward[key]
	if dirty {
		return value
	}

	value, cached := self.originReward[key]
	if cached {
		return value
	}

	k, _ := rlp.EncodeToBytes(key)
	enc, err := self.getRewardTrie(db).TryGet(k)
	if err != nil {
		self.setError(err)
		return nil
	}
	if len(enc) > 0 {
		value = new(big.Int)
		err := rlp.DecodeBytes(enc, value)
		if err != nil {
			self.setError(err)
		}
	}
	self.originReward[key] = value
	return value
}

func (self *stateObject) SetDelegateRewardBalance(db Database, key common.Address, rewardAmount *big.Int) {
	self.db.journal = append(self.db.journal, delegateRewardBalanceChange{
		account:  &self.address,
		key:      key,
		prevalue: self.GetDelegateRewardBalance(db, key),
	})
	self.setDelegateRewardBalance(key, rewardAmount)
}

func (self *stateObject) setDelegateRewardBalance(key common.Address, rewardAmount *big.Int) {
	self.dirtyReward[key] = rewardAmount

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) updateRewardTrie(db Database) Trie {
	tr := self.getRewardTrie(db)
	for key, value := range self.dirtyReward {
		delete(self.dirtyReward, key)

		if self.originReward[key] != nil && value.Cmp(self.originReward[key]) == 0 {
			continue
		}
		self.originReward[key] = value

		k, _ := rlp.EncodeToBytes(key)
		if value.Sign() == 0 {
			self.setError(tr.TryDelete(k))
			continue
		}

		v, _ := rlp.EncodeToBytes(value)
		self.setError(tr.TryUpdate(k, v))
	}
	return tr
}

func (self *stateObject) updateRewardRoot(db Database) {
	self.updateRewardTrie(db)
	self.data.RewardRoot = self.rewardTrie.Hash()
}

func (self *stateObject) CommitRewardTrie(db Database) error {
	self.updateRewardTrie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.rewardTrie.Commit(nil)
	if err == nil {
		self.data.RewardRoot = root
	}
	return err
}
