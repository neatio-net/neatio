package types

import (
	"time"

	. "github.com/nio-net/common"
	"github.com/nio-net/crypto"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/common/hexutil"
)

type EpochApi struct {
	Number         hexutil.Uint64    `json:"number"`
	RewardPerBlock *hexutil.Big      `json:"rewardPerBlock"`
	StartBlock     hexutil.Uint64    `json:"startBlock"`
	EndBlock       hexutil.Uint64    `json:"endBlock"`
	StartTime      time.Time         `json:"startTime"`
	EndTime        time.Time         `json:"endEime"`
	Validators     []*EpochValidator `json:"validators"`
}

type EpochApiForConsole struct {
	Number         hexutil.Uint64              `json:"number"`
	RewardPerBlock *hexutil.Big                `json:"rewardPerBlock"`
	StartBlock     hexutil.Uint64              `json:"startBlock"`
	EndBlock       hexutil.Uint64              `json:"endBlock"`
	StartTime      time.Time                   `json:"startTime"`
	EndTime        time.Time                   `json:"endTime"`
	Validators     []*EpochValidatorForConsole `json:"validators"`
}

type EpochVotesApi struct {
	EpochNumber hexutil.Uint64           `json:"voteForEpoch"`
	StartBlock  hexutil.Uint64           `json:"startBlock"`
	EndBlock    hexutil.Uint64           `json:"endBlock"`
	Votes       []*EpochValidatorVoteApi `json:"votes"`
}

type EpochVotesApiForConsole struct {
	EpochNumber hexutil.Uint64                     `json:"voteForEpoch"`
	StartBlock  hexutil.Uint64                     `json:"startBlock"`
	EndBlock    hexutil.Uint64                     `json:"endBlock"`
	Votes       []*EpochValidatorVoteApiForConsole `json:"votes"`
}

type EpochValidatorVoteApi struct {
	EpochValidator
	Salt     string      `json:"salt"`
	VoteHash common.Hash `json:"voteHash"`
	TxHash   common.Hash `json:"txHash"`
}

type EpochValidatorVoteApiForConsole struct {
	EpochValidatorForConsole
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

type EpochValidatorForConsole struct {
	Address        string         `json:"address"`
	PubKey         string         `json:"publicKey"`
	Amount         *hexutil.Big   `json:"votingPower"`
	RemainingEpoch hexutil.Uint64 `json:"remainEpoch"`
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

type ValidatorStatus struct {
	IsBanned bool `json:"isBanned"`
}

type CandidateApi struct {
	CandidateList []string `json:"candidateList"`
}

type BannedApi struct {
	BannedList []string `json:"bannedList"`
}
