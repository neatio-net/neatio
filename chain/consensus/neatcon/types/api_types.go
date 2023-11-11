package types

import (
	"time"

	. "github.com/neatio-net/common-go"
	"github.com/neatio-net/crypto-go"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/common/hexutil"
)

type EpochApi struct {
	Number         hexutil.Uint64    `json:"number"`
	RewardPerBlock *hexutil.Big      `json:"rewardPerBlock"`
	StartBlock     hexutil.Uint64    `json:"startBlock"`
	EndBlock       hexutil.Uint64    `json:"endBlock"`
	StartTime      time.Time         `json:"startTime"`
	EndTime        time.Time         `json:"endTime"`
	Validators     []*EpochValidator `json:"validators"`
}

type EpochVotesApi struct {
	EpochNumber hexutil.Uint64           `json:"voteForEpoch"`
	StartBlock  hexutil.Uint64           `json:"startBlock"`
	EndBlock    hexutil.Uint64           `json:"endBlock"`
	Votes       []*EpochValidatorVoteApi `json:"votes"`
}

type EpochValidatorVoteApi struct {
	EpochValidator
	Salt     string      `json:"salt"`
	VoteHash common.Hash `json:"voteHash"`
	TxHash   common.Hash `json:"txHash"`
}

type EpochValidator struct {
	Address        common.Address `json:"address"`
	PubKey         string         `json:"publicKey"`
	Amount         *hexutil.Big   `json:"votingPower"`
	RemainingEpoch hexutil.Uint64 `json:"remainEpoch"`
}

type EpochCandidate struct {
	Address common.Address `json:"address"`
}

type NeatConExtraApi struct {
	ChainID         string         `json:"chainId"`
	Height          hexutil.Uint64 `json:"height"`
	Time            time.Time      `json:"time"`
	NeedToSave      bool           `json:"needToSave"`
	NeedToBroadcast bool           `json:"needToBroadcast"`
	EpochNumber     hexutil.Uint64 `json:"epochNumber"`
	SeenCommitHash  string         `json:"lastCommitHash"`
	ValidatorsHash  string         `json:"validatorsHash"`
	SeenCommit      *CommitApi     `json:"seenCommit"`
	EpochBytes      []byte         `json:"epochBytes"`
}

type CommitApi struct {
	BlockID BlockIDApi     `json:"blockID"`
	Height  hexutil.Uint64 `json:"height"`
	Round   int            `json:"round"`

	SignAggr crypto.BLSSignature `json:"signAggr"`
	BitArray *BitArray           `json:"bitArray"`
}

type BlockIDApi struct {
	Hash        string           `json:"hash"`
	PartsHeader PartSetHeaderApi `json:"parts"`
}

type PartSetHeaderApi struct {
	Total hexutil.Uint64 `json:"total"`
	Hash  string         `json:"hash"`
}

type ConsensusAggr struct {
	PublicKeys []string         `json:"publicKey"`
	Addresses  []common.Address `json:"address"`
}
