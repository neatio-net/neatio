package state

import (
	"math/big"

	"github.com/neatio-net/neatio/utilities/common"
)

type journalEntry interface {
	undo(*StateDB)
}

type journal []journalEntry

type (
	createObjectChange struct {
		account *common.Address
	}
	resetObjectChange struct {
		prev *stateObject
	}
	suicideChange struct {
		account     *common.Address
		prev        bool
		prevbalance *big.Int
	}

	balanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	depositBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	sideChainDepositBalanceChange struct {
		account *common.Address
		chainId string
		prev    *big.Int
	}
	chainBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	delegateBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	proxiedBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	depositProxiedBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	pendingRefundBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	rewardBalanceChange struct {
		account *common.Address
		prev    *big.Int
	}

	nonceChange struct {
		account *common.Address
		prev    uint64
	}
	storageChange struct {
		account       *common.Address
		key, prevalue common.Hash
	}
	addTX1Change struct {
		account *common.Address
		txHash  common.Hash
	}
	addTX3Change struct {
		account *common.Address
		txHash  common.Hash
	}
	accountProxiedBalanceChange struct {
		account  *common.Address
		key      common.Address
		prevalue *accountProxiedBalance
	}
	delegateRewardBalanceChange struct {
		account  *common.Address
		key      common.Address
		prevalue *big.Int
	}

	candidateChange struct {
		account *common.Address
		prev    bool
	}

	pubkeyChange struct {
		account *common.Address
		prev    string
	}

	commissionChange struct {
		account *common.Address
		prev    uint8
	}

	bannedChange struct {
		account *common.Address
		prev    bool
	}

	blockTimeChange struct {
		account *common.Address
		prev    *big.Int
	}

	bannedTimeChange struct {
		account *common.Address
		prev    *big.Int
	}

	fAddressChange struct {
		account *common.Address
		prev    common.Address
	}

	codeChange struct {
		account            *common.Address
		prevcode, prevhash []byte
	}

	refundChange struct {
		prev uint64
	}
	addLogChange struct {
		txhash common.Hash
	}
	addPreimageChange struct {
		hash common.Hash
	}
	touchChange struct {
		account   *common.Address
		prev      bool
		prevDirty bool
	}
)

func (ch createObjectChange) undo(s *StateDB) {
	delete(s.stateObjects, *ch.account)
	delete(s.stateObjectsDirty, *ch.account)
}

func (ch resetObjectChange) undo(s *StateDB) {
	s.setStateObject(ch.prev)
}

func (ch suicideChange) undo(s *StateDB) {
	obj := s.getStateObject(*ch.account)
	if obj != nil {
		obj.suicided = ch.prev
		obj.setBalance(ch.prevbalance)
	}
}

var ripemd = common.HexToAddress("0000000000000000000000000000000000000003")

func (ch touchChange) undo(s *StateDB) {
	if !ch.prev && *ch.account != ripemd {
		s.getStateObject(*ch.account).touched = ch.prev
		if !ch.prevDirty {
			delete(s.stateObjectsDirty, *ch.account)
		}
	}
}

func (ch balanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setBalance(ch.prev)
}

func (ch depositBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setDepositBalance(ch.prev)
}

func (ch sideChainDepositBalanceChange) undo(s *StateDB) {
	self := s.getStateObject(*ch.account)

	var index = -1
	for i := range self.data.SideChainDepositBalance {
		if self.data.SideChainDepositBalance[i].ChainId == ch.chainId {
			index = i
			break
		}
	}
	if index < 0 {
		self.data.SideChainDepositBalance = append(self.data.SideChainDepositBalance, &sideChainDepositBalance{
			ChainId:        ch.chainId,
			DepositBalance: new(big.Int),
		})
		index = len(self.data.SideChainDepositBalance) - 1
	}

	self.setSideChainDepositBalance(index, ch.prev)
}

func (ch chainBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setChainBalance(ch.prev)
}

func (ch delegateBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setDelegateBalance(ch.prev)
}

func (ch proxiedBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setProxiedBalance(ch.prev)
}

func (ch depositProxiedBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setDepositProxiedBalance(ch.prev)
}

func (ch pendingRefundBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setPendingRefundBalance(ch.prev)
}

func (ch rewardBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setRewardBalance(ch.prev)
}

func (ch nonceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setNonce(ch.prev)
}

func (ch codeChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setCode(common.BytesToHash(ch.prevhash), ch.prevcode)
}

func (ch storageChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setState(ch.key, ch.prevalue)
}

func (ch addTX1Change) undo(s *StateDB) {
	s.getStateObject(*ch.account).removeTX1(ch.txHash)
}

func (ch addTX3Change) undo(s *StateDB) {
	s.getStateObject(*ch.account).removeTX3(ch.txHash)
}

func (ch accountProxiedBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setAccountProxiedBalance(ch.key, ch.prevalue)
}

func (ch delegateRewardBalanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setDelegateRewardBalance(ch.key, ch.prevalue)
}

func (ch candidateChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setCandidate(ch.prev)
}

func (ch pubkeyChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setPubkey(ch.prev)
}

func (ch commissionChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setCommission(ch.prev)
}

func (ch fAddressChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setAddress(ch.prev)
}

func (ch refundChange) undo(s *StateDB) {
	s.refund = ch.prev
}

func (ch addLogChange) undo(s *StateDB) {
	logs := s.logs[ch.txhash]
	if len(logs) == 1 {
		delete(s.logs, ch.txhash)
	} else {
		s.logs[ch.txhash] = logs[:len(logs)-1]
	}
	s.logSize--
}

func (ch addPreimageChange) undo(s *StateDB) {
	delete(s.preimages, ch.hash)
}
