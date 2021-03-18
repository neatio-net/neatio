package types

import (
	"time"

	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/common/hexutil"
	. "github.com/neatlib/common-go"
	"github.com/neatlib/crypto-go"
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

// For console
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
	VoteHash common.Hash `json:"voteHash"` // VoteHash = Keccak256(Epoch Number + PubKey + Amount + Salt)
	TxHash   common.Hash `json:"txHash"`
}

type EpochValidatorVoteApiForConsole struct {
	EpochValidatorForConsole
	Salt     string      `json:"salt"`
	VoteHash common.Hash `json:"voteHash"` // VoteHash = Keccak256(Epoch Number + PubKey + Amount + Salt)
	TxHash   common.Hash `json:"txHash"`
}

type EpochValidator struct {
	Address        common.Address `json:"address"`
	PubKey         string         `json:"publicKey"`
	Amount         *hexutil.Big   `json:"votingPower"`
	RemainingEpoch hexutil.Uint64 `json:"remainEpoch"`
}

// For console
type EpochValidatorForConsole struct {
	Address        string         `json:"address"`
	PubKey         string         `json:"publicKey"`
	Amount         *hexutil.Big   `json:"votingPower"`
	RemainingEpoch hexutil.Uint64 `json:"remainEpoch"`
}

type NeatconExtraApi struct {
	ChainID         string         `json:"chainId"`
	Height          hexutil.Uint64 `json:"height"`
	Time            time.Time      `json:"time"`
	NeedToSave      bool           `json:"needToSave"`
	NeedToBroadcast bool           `json:"needToBroadcast"`
	EpochNumber     hexutil.Uint64 `json:"epochNumber"`
	SeenCommitHash  string         `json:"lastCommitHash"` // commit from validators from the last block
	ValidatorsHash  string         `json:"validatorsHash"` // validators for the current block
	SeenCommit      *CommitApi     `json:"seenCommit"`
	EpochBytes      []byte         `json:"epochBytes"`
}

type CommitApi struct {
	BlockID BlockIDApi     `json:"blockID"`
	Height  hexutil.Uint64 `json:"height"`
	Round   int            `json:"round"`

	// BLS signature aggregation to be added here
	SignAggr crypto.BLSSignature `json:"signAggr"`
	BitArray *BitArray           `json:"bitArray"`

	//// Volatile
	//hash []byte
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
