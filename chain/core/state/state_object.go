package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/crypto"
	"github.com/neatio-net/neatio/utilities/rlp"
)

var emptyCodeHash = crypto.Keccak256(nil)

type Code []byte

func (self Code) String() string {
	return string(self)
}

type Storage map[common.Hash]common.Hash

func (self Storage) String() (str string) {
	for key, value := range self {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (self Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range self {
		cpy[key] = value
	}

	return cpy
}

type stateObject struct {
	address  common.Address
	addrHash common.Hash
	data     Account
	db       *StateDB

	dbErr error

	trie Trie
	code Code

	originStorage Storage
	dirtyStorage  Storage

	tx1Trie Trie
	tx3Trie Trie

	dirtyTX1 map[common.Hash]struct{}
	dirtyTX3 map[common.Hash]struct{}

	proxiedTrie   Trie
	originProxied Proxied
	dirtyProxied  Proxied

	rewardTrie   Trie
	originReward Reward
	dirtyReward  Reward

	dirtyCode bool
	suicided  bool
	touched   bool
	deleted   bool
	onDirty   func(addr common.Address)
}

func (s *stateObject) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0 && bytes.Equal(s.data.CodeHash, emptyCodeHash) && s.data.DepositBalance.Sign() == 0 && len(s.data.SideChainDepositBalance) == 0 && s.data.ChainBalance.Sign() == 0 && s.data.DelegateBalance.Sign() == 0 && s.data.ProxiedBalance.Sign() == 0 && s.data.DepositProxiedBalance.Sign() == 0 && s.data.PendingRefundBalance.Sign() == 0
}

type Account struct {
	Nonce                   uint64
	Balance                 *big.Int
	DepositBalance          *big.Int
	SideChainDepositBalance []*sideChainDepositBalance
	ChainBalance            *big.Int
	Root                    common.Hash
	TX1Root                 common.Hash
	TX3Root                 common.Hash
	CodeHash                []byte

	DelegateBalance       *big.Int
	ProxiedBalance        *big.Int
	DepositProxiedBalance *big.Int
	PendingRefundBalance  *big.Int
	ProxiedRoot           common.Hash

	Candidate  bool
	Commission uint8

	Pubkey   string
	FAddress common.Address

	RewardBalance *big.Int

	RewardRoot common.Hash
}

func newObject(db *StateDB, address common.Address, data Account, onDirty func(addr common.Address)) *stateObject {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	if data.DepositBalance == nil {
		data.DepositBalance = new(big.Int)
	}
	if data.ChainBalance == nil {
		data.ChainBalance = new(big.Int)
	}

	if data.DelegateBalance == nil {
		data.DelegateBalance = new(big.Int)
	}
	if data.ProxiedBalance == nil {
		data.ProxiedBalance = new(big.Int)
	}
	if data.DepositProxiedBalance == nil {
		data.DepositProxiedBalance = new(big.Int)
	}
	if data.PendingRefundBalance == nil {
		data.PendingRefundBalance = new(big.Int)
	}

	if data.RewardBalance == nil {
		data.RewardBalance = new(big.Int)
	}

	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}
	return &stateObject{
		db:            db,
		address:       address,
		addrHash:      crypto.Keccak256Hash(address[:]),
		data:          data,
		originStorage: make(Storage),
		dirtyStorage:  make(Storage),
		dirtyTX1:      make(map[common.Hash]struct{}),
		dirtyTX3:      make(map[common.Hash]struct{}),
		originProxied: make(Proxied),
		dirtyProxied:  make(Proxied),
		originReward:  make(Reward),
		dirtyReward:   make(Reward),
		onDirty:       onDirty,
	}
}

func (c *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, c.data)
}

func (self *stateObject) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *stateObject) markSuicided() {
	self.suicided = true
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (c *stateObject) touch() {
	c.db.journal = append(c.db.journal, touchChange{
		account:   &c.address,
		prev:      c.touched,
		prevDirty: c.onDirty == nil,
	})
	if c.onDirty != nil {
		c.onDirty(c.Address())
		c.onDirty = nil
	}
	c.touched = true
}

func (c *stateObject) getTX1Trie(db Database) Trie {
	if c.tx1Trie == nil {
		var err error
		c.tx1Trie, err = db.OpenTX1Trie(c.addrHash, c.data.TX1Root)
		if err != nil {
			c.tx1Trie, _ = db.OpenTX1Trie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create TX1 trie: %v", err))
		}
	}
	return c.tx1Trie
}

func (self *stateObject) HasTX1(db Database, txHash common.Hash) bool {

	_, ok := self.dirtyTX1[txHash]
	if ok {
		return true
	}

	enc, err := self.getTX1Trie(db).TryGet(txHash[:])
	if err != nil {
		return false
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			self.setError(err)
			return false
		}
		if !bytes.Equal(content, txHash[:]) {
			self.setError(fmt.Errorf("content mismatch the tx hash"))
			return false
		}

		return true
	}

	return false
}

func (self *stateObject) AddTX1(db Database, txHash common.Hash) {
	self.db.journal = append(self.db.journal, addTX1Change{
		account: &self.address,
		txHash:  txHash,
	})
	self.addTX1(txHash)
}

func (self *stateObject) addTX1(txHash common.Hash) {
	self.dirtyTX1[txHash] = struct{}{}
}

func (self *stateObject) removeTX1(txHash common.Hash) {
	delete(self.dirtyTX1, txHash)
}

func (self *stateObject) updateTX1Trie(db Database) Trie {
	tr := self.getTX1Trie(db)

	for tx1 := range self.dirtyTX1 {
		delete(self.dirtyTX1, tx1)

		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(tx1[:], "\x00"))
		self.setError(tr.TryUpdate(tx1[:], v))
	}
	return tr
}

func (self *stateObject) updateTX1Root(db Database) {
	self.updateTX1Trie(db)
	self.data.TX1Root = self.tx1Trie.Hash()
}

func (self *stateObject) CommitTX1Trie(db Database) error {
	self.updateTX1Trie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.tx1Trie.Commit(nil)
	if err == nil {
		self.data.TX1Root = root
	}
	return err
}

func (c *stateObject) getTX3Trie(db Database) Trie {
	if c.tx3Trie == nil {
		var err error
		c.tx3Trie, err = db.OpenTX3Trie(c.addrHash, c.data.TX3Root)
		if err != nil {
			c.tx3Trie, _ = db.OpenTX3Trie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create TX3 trie: %v", err))
		}
	}
	return c.tx3Trie
}

func (self *stateObject) HasTX3(db Database, txHash common.Hash) bool {

	_, ok := self.dirtyTX3[txHash]
	if ok {
		return true
	}

	enc, err := self.getTX3Trie(db).TryGet(txHash[:])
	if err != nil {
		return false
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			self.setError(err)
			return false
		}
		if !bytes.Equal(content, txHash[:]) {
			self.setError(fmt.Errorf("content mismatch the tx hash"))
			return false
		}

		return true
	}

	return false
}

func (self *stateObject) AddTX3(db Database, txHash common.Hash) {
	self.db.journal = append(self.db.journal, addTX3Change{
		account: &self.address,
		txHash:  txHash,
	})
	self.addTX3(txHash)
}

func (self *stateObject) addTX3(txHash common.Hash) {
	self.dirtyTX3[txHash] = struct{}{}
}

func (self *stateObject) removeTX3(txHash common.Hash) {
	delete(self.dirtyTX3, txHash)
}

func (self *stateObject) updateTX3Trie(db Database) Trie {
	tr := self.getTX3Trie(db)

	for tx3 := range self.dirtyTX3 {
		delete(self.dirtyTX3, tx3)

		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(tx3[:], "\x00"))
		self.setError(tr.TryUpdate(tx3[:], v))
	}
	return tr
}

func (self *stateObject) updateTX3Root(db Database) {
	self.updateTX3Trie(db)
	self.data.TX3Root = self.tx3Trie.Hash()
}

func (self *stateObject) CommitTX3Trie(db Database) error {
	self.updateTX3Trie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.tx3Trie.Commit(nil)
	if err == nil {
		self.data.TX3Root = root
	}
	return err
}

func (c *stateObject) getTrie(db Database) Trie {
	if c.trie == nil {
		var err error
		c.trie, err = db.OpenStorageTrie(c.addrHash, c.data.Root)
		if err != nil {
			c.trie, _ = db.OpenStorageTrie(c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return c.trie
}

func (self *stateObject) GetState(db Database, key common.Hash) common.Hash {

	value, dirty := self.dirtyStorage[key]
	if dirty {
		return value
	}

	return self.GetCommittedState(db, key)
}

func (self *stateObject) GetCommittedState(db Database, key common.Hash) common.Hash {

	value, cached := self.originStorage[key]
	if cached {
		return value
	}

	enc, err := self.getTrie(db).TryGet(key[:])
	if err != nil {
		self.setError(err)
		return common.Hash{}
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			self.setError(err)
		}
		value.SetBytes(content)
	}
	self.originStorage[key] = value
	return value
}

func (self *stateObject) SetState(db Database, key, value common.Hash) {
	self.db.journal = append(self.db.journal, storageChange{
		account:  &self.address,
		key:      key,
		prevalue: self.GetState(db, key),
	})
	self.setState(key, value)
}

func (self *stateObject) setState(key, value common.Hash) {
	self.dirtyStorage[key] = value

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) updateTrie(db Database) Trie {
	tr := self.getTrie(db)
	for key, value := range self.dirtyStorage {
		delete(self.dirtyStorage, key)

		if value == self.originStorage[key] {
			continue
		}
		self.originStorage[key] = value

		if (value == common.Hash{}) {
			self.setError(tr.TryDelete(key[:]))
			continue
		}

		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		self.setError(tr.TryUpdate(key[:], v))
	}
	return tr
}

func (self *stateObject) updateRoot(db Database) {
	self.updateTrie(db)
	self.data.Root = self.trie.Hash()
}

func (self *stateObject) CommitTrie(db Database) error {
	self.updateTrie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.trie.Commit(nil)
	if err == nil {
		self.data.Root = root
	}
	return err
}

func (c *stateObject) AddBalance(amount *big.Int) {

	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}

		return
	}
	c.SetBalance(new(big.Int).Add(c.Balance(), amount))
}

func (c *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	c.SetBalance(new(big.Int).Sub(c.Balance(), amount))
}

func (self *stateObject) SetBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, balanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.Balance),
	})
	self.setBalance(amount)
}

func (self *stateObject) setBalance(amount *big.Int) {
	self.data.Balance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (c *stateObject) ReturnGas(gas *big.Int) {}

func (self *stateObject) deepCopy(db *StateDB, onDirty func(addr common.Address)) *stateObject {
	stateObject := newObject(db, self.address, self.data, onDirty)
	if self.trie != nil {
		stateObject.trie = db.db.CopyTrie(self.trie)
	}
	if self.tx1Trie != nil {
		stateObject.tx1Trie = db.db.CopyTrie(self.tx1Trie)
	}
	if self.tx3Trie != nil {
		stateObject.tx3Trie = db.db.CopyTrie(self.tx3Trie)
	}
	if self.proxiedTrie != nil {
		stateObject.proxiedTrie = db.db.CopyTrie(self.proxiedTrie)
	}
	if self.rewardTrie != nil {
		stateObject.rewardTrie = db.db.CopyTrie(self.rewardTrie)
	}
	stateObject.code = self.code
	stateObject.dirtyStorage = self.dirtyStorage.Copy()
	stateObject.originStorage = self.originStorage.Copy()
	stateObject.suicided = self.suicided
	stateObject.dirtyCode = self.dirtyCode
	stateObject.deleted = self.deleted
	stateObject.dirtyTX1 = make(map[common.Hash]struct{})
	for tx1 := range self.dirtyTX1 {
		stateObject.dirtyTX1[tx1] = struct{}{}
	}
	stateObject.dirtyTX3 = make(map[common.Hash]struct{})
	for tx3 := range self.dirtyTX3 {
		stateObject.dirtyTX3[tx3] = struct{}{}
	}
	stateObject.dirtyProxied = self.dirtyProxied.Copy()
	stateObject.originProxied = self.originProxied.Copy()
	stateObject.dirtyReward = self.dirtyReward.Copy()
	stateObject.originReward = self.originReward.Copy()
	return stateObject
}

func (c *stateObject) Address() common.Address {
	return c.address
}

func (self *stateObject) Code(db Database) []byte {
	if self.code != nil {
		return self.code
	}
	if bytes.Equal(self.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(self.addrHash, common.BytesToHash(self.CodeHash()))
	if err != nil {
		self.setError(fmt.Errorf("can't load code hash %x: %v", self.CodeHash(), err))
	}
	self.code = code
	return code
}

func (self *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := self.Code(self.db.db)
	self.db.journal = append(self.db.journal, codeChange{
		account:  &self.address,
		prevhash: self.CodeHash(),
		prevcode: prevcode,
	})
	self.setCode(codeHash, code)
}

func (self *stateObject) setCode(codeHash common.Hash, code []byte) {
	self.code = code
	self.data.CodeHash = codeHash[:]
	self.dirtyCode = true
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) SetNonce(nonce uint64) {
	self.db.journal = append(self.db.journal, nonceChange{
		account: &self.address,
		prev:    self.data.Nonce,
	})
	self.setNonce(nonce)
}

func (self *stateObject) setNonce(nonce uint64) {
	self.data.Nonce = nonce
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) CodeHash() []byte {
	return self.data.CodeHash
}

func (self *stateObject) Balance() *big.Int {
	return self.data.Balance
}

func (self *stateObject) Nonce() uint64 {
	return self.data.Nonce
}

func (self *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}

func (self *stateObject) SetAddress(address common.Address) {
	self.db.journal = append(self.db.journal, fAddressChange{
		account: &self.address,
		prev:    self.data.FAddress,
	})

	self.setAddress(address)
}

func (self *stateObject) setAddress(address common.Address) {
	self.data.FAddress = address
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) GetAddress() common.Address {
	return self.data.FAddress
}
