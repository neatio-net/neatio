package types

import (
	"fmt"
	"math/big"

	"github.com/nio-net/crypto"
	"github.com/nio-net/nio/utilities/common"
)

// PendingOps tracks the operations(except balance related stuff since it's tracked in statedb) that need to be applied after consensus achieved.
type PendingOps struct {
	ops []PendingOp
}

func (pending *PendingOps) Append(op1 PendingOp) bool {
	for _, op := range pending.ops {
		if op.Conflict(op1) {
			return false
		}
	}
	pending.ops = append(pending.ops, op1)
	return true
}

func (pending *PendingOps) Ops() []PendingOp {
	ret := make([]PendingOp, len(pending.ops))
	copy(ret, pending.ops)
	return ret
}

type PendingOp interface {
	Conflict(op PendingOp) bool
	String() string
}

// CreateSideChain op
type CreateSideChainOp struct {
	From             common.Address
	ChainId          string
	MinValidators    uint16
	MinDepositAmount *big.Int
	StartBlock       *big.Int
	EndBlock         *big.Int
}

func (op *CreateSideChainOp) Conflict(op1 PendingOp) bool {
	if op1, ok := op1.(*CreateSideChainOp); ok {
		return op.ChainId == op1.ChainId
	}
	return false
}

func (op *CreateSideChainOp) String() string {
	return fmt.Sprintf("CreateSideChainOp - From: %x, ChainId: %s, MinValidators: %d, MinDepositAmount: %x, StartBlock: %x, EndBlock: %x",
		op.From, op.ChainId, op.MinValidators, op.MinDepositAmount, op.StartBlock, op.EndBlock)
}

// JoinSideChain op
type JoinSideChainOp struct {
	From          common.Address
	PubKey        crypto.PubKey
	ChainId       string
	DepositAmount *big.Int
}

func (op *JoinSideChainOp) Conflict(op1 PendingOp) bool {
	if op1, ok := op1.(*JoinSideChainOp); ok {
		return op.ChainId == op1.ChainId && op.From == op1.From
	}
	return false
}

func (op *JoinSideChainOp) String() string {
	return fmt.Sprintf("JoinSideChainOp - From: %x, PubKey: %s, ChainId: %s, DepositAmount: %x",
		op.From, op.PubKey, op.ChainId, op.DepositAmount)
}

// LaunchSideChain op
type LaunchSideChainsOp struct {
	SideChainIds       []string
	NewPendingIdx      []byte
	DeleteSideChainIds []string
}

func (op *LaunchSideChainsOp) Conflict(op1 PendingOp) bool {
	if _, ok := op1.(*LaunchSideChainsOp); ok {
		// Only one LaunchSideChainsOp is allowed in each block
		return true
	}
	return false
}

func (op *LaunchSideChainsOp) String() string {
	return fmt.Sprintf("LaunchSideChainsOp - Launch Side Chain: %v, New Pending Side Chain Length: %v, To be deleted Side Chain: %v",
		op.SideChainIds, len(op.NewPendingIdx), op.DeleteSideChainIds)
}

// SaveBlockToMainChain op
type SaveDataToMainChainOp struct {
	Data []byte
}

func (op *SaveDataToMainChainOp) Conflict(op1 PendingOp) bool {
	return false
}

func (op *SaveDataToMainChainOp) String() string {
	return fmt.Sprintf("SaveDataToMainChainOp")
}

// VoteNextEpoch op
type VoteNextEpochOp struct {
	From     common.Address
	VoteHash common.Hash
	TxHash   common.Hash
}

func (op *VoteNextEpochOp) Conflict(op1 PendingOp) bool {
	return false
}

func (op *VoteNextEpochOp) String() string {
	return fmt.Sprintf("VoteNextEpoch")
}

// RevealVote op
type RevealVoteOp struct {
	From   common.Address
	Pubkey crypto.PubKey
	Amount *big.Int
	Salt   string
	TxHash common.Hash
}

func (op *RevealVoteOp) Conflict(op1 PendingOp) bool {
	return false
}

func (op *RevealVoteOp) String() string {
	return fmt.Sprintf("RevealVote")
}

type UpdateNextEpochOp struct {
	From   common.Address
	PubKey crypto.PubKey
	Amount *big.Int
	Salt   string
	TxHash common.Hash
}

func (op *UpdateNextEpochOp) Conflict(op1 PendingOp) bool {
	return false
}

func (op *UpdateNextEpochOp) String() string {
	return fmt.Sprintf("UpdateNextEpoch")
}
