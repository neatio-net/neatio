package state

import (
	"math/big"

	"github.com/nio-net/nio/utilities/common"
)

// sideChainDepositBalance
type sideChainDepositBalance struct {
	ChainId        string
	DepositBalance *big.Int
}

// AddDepositBalance add amount to the deposit balance.
func (c *stateObject) AddDepositBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Cmp(common.Big0) == 0 {
		if c.empty() {
			c.touch()
		}

		return
	}

	c.SetDepositBalance(new(big.Int).Add(c.DepositBalance(), amount))
}

// SubDepositBalance removes amount from c's deposit balance.
func (c *stateObject) SubDepositBalance(amount *big.Int) {
	if amount.Cmp(common.Big0) == 0 {
		return
	}
	c.SetDepositBalance(new(big.Int).Sub(c.DepositBalance(), amount))
}

func (self *stateObject) SetDepositBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, depositBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.DepositBalance),
	})
	self.setDepositBalance(amount)
}

func (self *stateObject) setDepositBalance(amount *big.Int) {
	self.data.DepositBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

// AddSideChainDepositBalance add amount to the side chain deposit balance
func (c *stateObject) AddSideChainDepositBalance(chainId string, amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Cmp(common.Big0) == 0 {
		if c.empty() {
			c.touch()
		}

		return
	}

	c.SetSideChainDepositBalance(chainId, new(big.Int).Add(c.SideChainDepositBalance(chainId), amount))
}

// SubSideChainDepositBalance removes amount from c's side chain deposit balance
func (c *stateObject) SubSideChainDepositBalance(chainId string, amount *big.Int) {
	if amount.Cmp(common.Big0) == 0 {
		return
	}
	c.SetSideChainDepositBalance(chainId, new(big.Int).Sub(c.SideChainDepositBalance(chainId), amount))
}

func (self *stateObject) SetSideChainDepositBalance(chainId string, amount *big.Int) {
	var index = -1
	for i := range self.data.SideChainDepositBalance {
		if self.data.SideChainDepositBalance[i].ChainId == chainId {
			index = i
			break
		}
	}
	if index < 0 { // not found, we'll append
		self.data.SideChainDepositBalance = append(self.data.SideChainDepositBalance, &sideChainDepositBalance{
			ChainId:        chainId,
			DepositBalance: new(big.Int),
		})
		index = len(self.data.SideChainDepositBalance) - 1
	}

	self.db.journal = append(self.db.journal, sideChainDepositBalanceChange{
		account: &self.address,
		chainId: chainId,
		prev:    new(big.Int).Set(self.data.SideChainDepositBalance[index].DepositBalance),
	})
	self.setSideChainDepositBalance(index, amount)
}

func (self *stateObject) setSideChainDepositBalance(index int, amount *big.Int) {
	self.data.SideChainDepositBalance[index].DepositBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

// AddChainBalance add amount to the locked balance.
func (c *stateObject) AddChainBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Cmp(common.Big0) == 0 {
		if c.empty() {
			c.touch()
		}

		return
	}

	c.SetChainBalance(new(big.Int).Add(c.ChainBalance(), amount))
}

// SubChainBalance removes amount from c's chain balance.
func (c *stateObject) SubChainBalance(amount *big.Int) {
	if amount.Cmp(common.Big0) == 0 {
		return
	}
	c.SetChainBalance(new(big.Int).Sub(c.ChainBalance(), amount))
}

func (self *stateObject) SetChainBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, chainBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.ChainBalance),
	})
	self.setChainBalance(amount)
}

func (self *stateObject) setChainBalance(amount *big.Int) {
	self.data.ChainBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) DepositBalance() *big.Int {
	return self.data.DepositBalance
}

func (self *stateObject) SideChainDepositBalance(chainId string) *big.Int {
	for i := range self.data.SideChainDepositBalance {
		if self.data.SideChainDepositBalance[i].ChainId == chainId {
			return self.data.SideChainDepositBalance[i].DepositBalance
		}
	}
	return common.Big0
}

func (self *stateObject) ChainBalance() *big.Int {
	return self.data.ChainBalance
}
