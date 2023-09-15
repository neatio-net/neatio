package state

import (
	"fmt"
	"math/big"

	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/rlp"
)

// ----- Type
type accountProxiedBalance struct {
	ProxiedBalance        *big.Int
	DepositProxiedBalance *big.Int
	PendingRefundBalance  *big.Int
}

func (a *accountProxiedBalance) String() (str string) {
	return fmt.Sprintf("pb: %v, dpb: %v, rb: %v", a.ProxiedBalance, a.DepositProxiedBalance, a.PendingRefundBalance)
}

func (a *accountProxiedBalance) Copy() *accountProxiedBalance {
	cpy := *a
	return &cpy
}

func (a *accountProxiedBalance) Equal(b *accountProxiedBalance) bool {
	if b == nil {
		return false
	}
	return a.ProxiedBalance.Cmp(b.ProxiedBalance) == 0 && a.DepositProxiedBalance.Cmp(b.DepositProxiedBalance) == 0 && a.PendingRefundBalance.Cmp(b.PendingRefundBalance) == 0
}

func (a *accountProxiedBalance) IsEmpty() bool {
	return a.ProxiedBalance.Sign() == 0 && a.DepositProxiedBalance.Sign() == 0 && a.PendingRefundBalance.Sign() == 0
}

func NewAccountProxiedBalance() *accountProxiedBalance {
	return &accountProxiedBalance{
		ProxiedBalance:        big.NewInt(0),
		DepositProxiedBalance: big.NewInt(0),
		PendingRefundBalance:  big.NewInt(0),
	}
}

type Proxied map[common.Address]*accountProxiedBalance

func (p Proxied) String() (str string) {
	for key, value := range p {
		str += fmt.Sprintf("%v : %X\n", key.String(), value)
	}
	return
}

func (p Proxied) Copy() Proxied {
	cpy := make(Proxied)
	for key, value := range p {
		cpy[key] = value.Copy()
	}
	return cpy
}

// ----- DelegateBalance

// AddDelegateBalance add amount to c's DelegateBalance.
func (c *stateObject) AddDelegateBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}
		return
	}
	c.SetDelegateBalance(new(big.Int).Add(c.DelegateBalance(), amount))
}

// SubDelegateBalance removes amount from c's DelegateBalance.
func (c *stateObject) SubDelegateBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetDelegateBalance(new(big.Int).Sub(c.DelegateBalance(), amount))
}

func (self *stateObject) SetDelegateBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, delegateBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.DelegateBalance),
	})
	self.setDelegateBalance(amount)
}

func (self *stateObject) setDelegateBalance(amount *big.Int) {
	self.data.DelegateBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) DelegateBalance() *big.Int {
	return self.data.DelegateBalance
}

// ----- ProxiedBalance

// AddProxiedBalance add amount to c's ProxiedBalance.
func (c *stateObject) AddProxiedBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}
		return
	}
	c.SetProxiedBalance(new(big.Int).Add(c.ProxiedBalance(), amount))
}

// SubProxiedBalance removes amount from c's ProxiedBalance.
func (c *stateObject) SubProxiedBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetProxiedBalance(new(big.Int).Sub(c.ProxiedBalance(), amount))
}

func (self *stateObject) SetProxiedBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, proxiedBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.ProxiedBalance),
	})
	self.setProxiedBalance(amount)
}

func (self *stateObject) setProxiedBalance(amount *big.Int) {
	self.data.ProxiedBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) ProxiedBalance() *big.Int {
	return self.data.ProxiedBalance
}

// ----- DepositProxiedBalance

// AddDepositProxiedBalance add amount to c's DepositProxiedBalance.
func (c *stateObject) AddDepositProxiedBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}
		return
	}
	c.SetDepositProxiedBalance(new(big.Int).Add(c.DepositProxiedBalance(), amount))
}

// SubDepositProxiedBalance removes amount from c's DepositProxiedBalance.
func (c *stateObject) SubDepositProxiedBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetDepositProxiedBalance(new(big.Int).Sub(c.DepositProxiedBalance(), amount))
}

func (self *stateObject) SetDepositProxiedBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, depositProxiedBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.DepositProxiedBalance),
	})
	self.setDepositProxiedBalance(amount)
}

func (self *stateObject) setDepositProxiedBalance(amount *big.Int) {
	self.data.DepositProxiedBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) DepositProxiedBalance() *big.Int {
	return self.data.DepositProxiedBalance
}

// ----- PendingRefundBalance

// AddPendingRefundBalance add amount to c's PendingRefundBalance.
func (c *stateObject) AddPendingRefundBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}
		return
	}
	c.SetPendingRefundBalance(new(big.Int).Add(c.PendingRefundBalance(), amount))
}

// SubPendingRefundBalance removes amount from c's PendingRefundBalance.
func (c *stateObject) SubPendingRefundBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetPendingRefundBalance(new(big.Int).Sub(c.PendingRefundBalance(), amount))
}

func (self *stateObject) SetPendingRefundBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, pendingRefundBalanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.PendingRefundBalance),
	})
	self.setPendingRefundBalance(amount)
}

func (self *stateObject) setPendingRefundBalance(amount *big.Int) {
	self.data.PendingRefundBalance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) PendingRefundBalance() *big.Int {
	return self.data.PendingRefundBalance
}

// ----- Delegate Trie

func (c *stateObject) getProxiedTrie(db Database) Trie {
	if c.proxiedTrie == nil {
		var err error
		c.proxiedTrie, err = db.OpenProxiedTrie(c.addrHash, c.data.ProxiedRoot)
		if err != nil {
			c.proxiedTrie, _ = db.OpenProxiedTrie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create proxied trie: %v", err))
		}
	}
	return c.proxiedTrie
}

// GetAccountProxiedBalance returns a value in proxied trie
func (self *stateObject) GetAccountProxiedBalance(db Database, key common.Address) *accountProxiedBalance {
	// If we have a dirty value for this state entry, return it
	value, dirty := self.dirtyProxied[key]
	if dirty {
		return value
	}
	// If we have the original value cached, return that
	value, cached := self.originProxied[key]
	if cached {
		return value
	}
	// Otherwise load the value from the database
	enc, err := self.getProxiedTrie(db).TryGet(key[:])
	if err != nil {
		self.setError(err)
		return nil
	}
	if len(enc) > 0 {
		value = new(accountProxiedBalance)
		err := rlp.DecodeBytes(enc, value)
		if err != nil {
			self.setError(err)
		}
	}
	self.originProxied[key] = value
	return value
}

// SetAccountProxiedBalance updates a value in account storage.
func (self *stateObject) SetAccountProxiedBalance(db Database, key common.Address, proxiedBalance *accountProxiedBalance) {
	self.db.journal = append(self.db.journal, accountProxiedBalanceChange{
		account:  &self.address,
		key:      key,
		prevalue: self.GetAccountProxiedBalance(db, key),
	})
	self.setAccountProxiedBalance(key, proxiedBalance)
}

func (self *stateObject) setAccountProxiedBalance(key common.Address, proxiedBalance *accountProxiedBalance) {
	self.dirtyProxied[key] = proxiedBalance

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

// updateProxiedTrie writes cached proxied modifications into the object's proxied trie.
func (self *stateObject) updateProxiedTrie(db Database) Trie {
	tr := self.getProxiedTrie(db)
	for key, value := range self.dirtyProxied {
		delete(self.dirtyProxied, key)

		// Skip noop changes, persist actual changes
		if value.Equal(self.originProxied[key]) {
			continue
		}
		self.originProxied[key] = value

		if value.IsEmpty() {
			self.setError(tr.TryDelete(key[:]))
			continue
		}
		// Encoding []byte cannot fail, ok to ignore the error.
		v, _ := rlp.EncodeToBytes(value)
		self.setError(tr.TryUpdate(key[:], v))
	}
	return tr
}

// updateProxiedRoot sets the proxiedTrie root to the current root hash of
func (self *stateObject) updateProxiedRoot(db Database) {
	self.updateProxiedTrie(db)
	self.data.ProxiedRoot = self.proxiedTrie.Hash()
}

// CommitProxiedTrie the proxied trie of the object to dwb.
// This updates the proxied trie root.
func (self *stateObject) CommitProxiedTrie(db Database) error {
	self.updateProxiedTrie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.proxiedTrie.Commit(nil)
	if err == nil {
		self.data.ProxiedRoot = root
	}
	return err
}

func (self *stateObject) IsEmptyTrie() bool {
	return self.data.ProxiedRoot == types.EmptyRootHash
}

// ----- Candidate

func (self *stateObject) IsCandidate() bool {
	return self.data.Candidate
}

func (self *stateObject) SetCandidate(isCandidate bool) {
	self.db.journal = append(self.db.journal, candidateChange{
		account: &self.address,
		prev:    self.data.Candidate,
	})
	self.setCandidate(isCandidate)
}

func (self *stateObject) setCandidate(isCandidate bool) {
	self.data.Candidate = isCandidate

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) Pubkey() string {
	return self.data.Pubkey
}

func (self *stateObject) SetPubkey(pubkey string) {
	self.db.journal = append(self.db.journal, pubkeyChange{
		account: &self.address,
		prev:    self.data.Pubkey,
	})

	self.setPubkey(pubkey)
}

func (self *stateObject) setPubkey(pubkey string) {
	self.data.Pubkey = pubkey

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) Commission() uint8 {
	return self.data.Commission
}

func (self *stateObject) SetCommission(commission uint8) {
	self.db.journal = append(self.db.journal, commissionChange{
		account: &self.address,
		prev:    self.data.Commission,
	})
	self.setCommission(commission)
}

func (self *stateObject) setCommission(commission uint8) {
	self.data.Commission = commission

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) IsBanned() bool {
	return self.data.IsBanned
}

func (self *stateObject) SetBanned(banned bool) {
	self.db.journal = append(self.db.journal, bannedChange{
		account: &self.address,
		prev:    self.data.IsBanned,
	})
	self.setBanned(banned)
}

func (self *stateObject) setBanned(banned bool) {
	self.data.IsBanned = banned

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) BlockTime() *big.Int {
	return self.data.BlockTime
}

func (self *stateObject) SetBlockTime(blockTime *big.Int) {
	self.db.journal = append(self.db.journal, blockTimeChange{
		account: &self.address,
		prev:    self.data.BlockTime,
	})
	self.setBlockTime(blockTime)
}

func (self *stateObject) setBlockTime(blockTime *big.Int) {
	self.data.BlockTime = blockTime

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) BannedTime() *big.Int {
	return self.data.BannedTime
}

func (self *stateObject) SetBannedTime(bannedTime *big.Int) {
	self.db.journal = append(self.db.journal, bannedTimeChange{
		account: &self.address,
		prev:    self.data.BannedTime,
	})
	self.setBannedTime(bannedTime)
}

func (self *stateObject) setBannedTime(bannedTime *big.Int) {
	self.data.BannedTime = bannedTime

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}
