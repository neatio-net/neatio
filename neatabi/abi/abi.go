package abi

import (
	"math/big"
	"strings"

	"github.com/neatlab/neatio/chain/accounts/abi"
	"github.com/neatlab/neatio/utilities/common"
)

type FunctionType struct {
	id    int
	cross bool
	main  bool
	side  bool
}

var (
	CreateSideChain       = FunctionType{0, true, true, false}
	JoinSideChain         = FunctionType{1, true, true, false}
	DepositInMainChain    = FunctionType{2, true, true, false}
	DepositInSideChain    = FunctionType{3, true, false, true}
	WithdrawFromSideChain = FunctionType{4, true, false, true}
	WithdrawFromMainChain = FunctionType{5, true, true, false}
	SaveDataToMainChain   = FunctionType{6, true, true, false}
	SetBlockReward        = FunctionType{7, true, false, true}

	VoteNextEpoch  = FunctionType{10, false, true, true}
	RevealVote     = FunctionType{11, false, true, true}
	Delegate       = FunctionType{12, false, true, true}
	UnDelegate     = FunctionType{13, false, true, true}
	Register       = FunctionType{14, false, true, true}
	UnRegister     = FunctionType{15, false, true, true}
	EditValidator  = FunctionType{16, false, true, true}
	WithdrawReward = FunctionType{17, false, true, true}
	UnBanned       = FunctionType{18, false, true, true}
	SetCommission  = FunctionType{19, false, true, true}

	Unknown = FunctionType{-1, false, false, false}
)

func (t FunctionType) IsCrossChainType() bool {
	return t.cross
}

func (t FunctionType) AllowInMainChain() bool {
	return t.main
}

func (t FunctionType) AllowInSideChain() bool {
	return t.side
}

func (t FunctionType) RequiredGas() uint64 {
	switch t {
	case CreateSideChain:
		return 200000
	case JoinSideChain:
		return 100000
	case DepositInMainChain:
		return 200000
	case DepositInSideChain:
		return 0
	case WithdrawFromSideChain:
		return 200000
	case WithdrawFromMainChain:
		return 0
	case SaveDataToMainChain:
		return 0
	case VoteNextEpoch:
		return 100000
	case RevealVote:
		return 100000
	case Delegate, UnDelegate, Register, UnRegister:
		return 100000
	case SetBlockReward:
		return 100000
	case EditValidator:
		return 100000
	case WithdrawReward:
		return 100000
	case UnBanned:
		return 100000
	case SetCommission:
		return 100000
	default:
		return 0
	}
}

func (t FunctionType) String() string {
	switch t {
	case CreateSideChain:
		return "CreateSideChain"
	case JoinSideChain:
		return "JoinSideChain"
	case DepositInMainChain:
		return "DepositInMainChain"
	case DepositInSideChain:
		return "DepositInSideChain"
	case WithdrawFromSideChain:
		return "WithdrawFromSideChain"
	case WithdrawFromMainChain:
		return "WithdrawFromMainChain"
	case SaveDataToMainChain:
		return "SaveDataToMainChain"
	case VoteNextEpoch:
		return "VoteNextEpoch"
	case RevealVote:
		return "RevealVote"
	case Delegate:
		return "Delegate"
	case UnDelegate:
		return "UnDelegate"
	case Register:
		return "Register"
	case UnRegister:
		return "UnRegister"
	case SetBlockReward:
		return "SetBlockReward"
	case EditValidator:
		return "EditValidator"
	case WithdrawReward:
		return "WithdrawReward"
	case UnBanned:
		return "UnBanned"
	case SetCommission:
		return "SetCommission"
	default:
		return "UnKnown"
	}
}

func StringToFunctionType(s string) FunctionType {
	switch s {
	case "CreateSideChain":
		return CreateSideChain
	case "JoinSideChain":
		return JoinSideChain
	case "DepositInMainChain":
		return DepositInMainChain
	case "DepositInSideChain":
		return DepositInSideChain
	case "WithdrawFromSideChain":
		return WithdrawFromSideChain
	case "WithdrawFromMainChain":
		return WithdrawFromMainChain
	case "SaveDataToMainChain":
		return SaveDataToMainChain
	case "VoteNextEpoch":
		return VoteNextEpoch
	case "RevealVote":
		return RevealVote
	case "Delegate":
		return Delegate
	case "UnDelegate":
		return UnDelegate
	case "Register":
		return Register
	case "UnRegister":
		return UnRegister
	case "SetBlockReward":
		return SetBlockReward
	case "EditValidator":
		return EditValidator
	case "WithdrawReward":
		return WithdrawReward
	case "UnBanned":
		return UnBanned
	case "SetCommission":
		return SetCommission
	default:
		return Unknown
	}
}

type CreateSideChainArgs struct {
	ChainId          string
	MinValidators    uint16
	MinDepositAmount *big.Int
	StartBlock       *big.Int
	EndBlock         *big.Int
}

type JoinSideChainArgs struct {
	PubKey    []byte
	ChainId   string
	Signature []byte
}

type DepositInMainChainArgs struct {
	ChainId string
}

type DepositInSideChainArgs struct {
	ChainId string
	TxHash  common.Hash
}

type WithdrawFromSideChainArgs struct {
	ChainId string
}

type WithdrawFromMainChainArgs struct {
	ChainId string
	Amount  *big.Int
	TxHash  common.Hash
}

type VoteNextEpochArgs struct {
	VoteHash common.Hash
}

type RevealVoteArgs struct {
	PubKey    []byte
	Amount    *big.Int
	Salt      string
	Signature []byte
}

type DelegateArgs struct {
	Candidate common.Address
}

type UnDelegateArgs struct {
	Candidate common.Address
	Amount    *big.Int
}

type RegisterArgs struct {
	Pubkey     []byte
	Signature  []byte
	Commission uint8
}

type SetBlockRewardArgs struct {
	ChainId string
	Reward  *big.Int
}

type EditValidatorArgs struct {
	Moniker  string
	Website  string
	Identity string
	Details  string
}

type WithdrawRewardArgs struct {
	DelegateAddress common.Address
}

type UnBannedArgs struct {
}

type SetCommissionArgs struct {
	Commission uint8
}

const jsonChainABI = `
[
	{
		"type": "function",
		"name": "CreateSideChain",
		"constant": false,
		"inputs": [
			{
				"name": "chainId",
				"type": "string"
			},
			{
				"name": "minValidators",
				"type": "uint16"
			},
			{
				"name": "minDepositAmount",
				"type": "uint256"
			},
			{
				"name": "startBlock",
				"type": "uint256"
			},
			{
				"name": "endBlock",
				"type": "uint256"
			}
		]
	},
	{
		"type": "function",
		"name": "JoinSideChain",
		"constant": false,
		"inputs": [
			{
				"name": "pubKey",
				"type": "bytes"
			},
			{
				"name": "chainId",
				"type": "string"
			},
			{
				"name": "signature",
				"type": "bytes"
			}
		]
	},
	{
		"type": "function",
		"name": "DepositInMainChain",
		"constant": false,
		"inputs": [
			{
				"name": "chainId",
				"type": "string"
			}
		]
	},
	{
		"type": "function",
		"name": "DepositInSideChain",
		"constant": false,
		"inputs": [
			{
				"name": "chainId",
				"type": "string"
			},
			{
				"name": "txHash",
				"type": "bytes32"
			}
		]
	},
	{
		"type": "function",
		"name": "WithdrawFromSideChain",
		"constant": false,
		"inputs": [
			{
				"name": "chainId",
				"type": "string"
			}
		]
	},
	{
		"type": "function",
		"name": "WithdrawFromMainChain",
		"constant": false,
		"inputs": [
			{
				"name": "chainId",
				"type": "string"
			},
			{
				"name": "amount",
				"type": "uint256"
			},
			{
				"name": "txHash",
				"type": "bytes32"
			}
		]
	},
	{
		"type": "function",
		"name": "SaveDataToMainChain",
		"constant": false,
		"inputs": [
			{
				"name": "data",
				"type": "bytes"
			}
		]
	},
	{
		"type": "function",
		"name": "VoteNextEpoch",
		"constant": false,
		"inputs": [
			{
				"name": "voteHash",
				"type": "bytes32"
			}
		]
	},
	{
		"type": "function",
		"name": "RevealVote",
		"constant": false,
		"inputs": [
			{
				"name": "pubKey",
				"type": "bytes"
			},
			{
				"name": "amount",
				"type": "uint256"
			},
			{
				"name": "salt",
				"type": "string"
			},
			{
				"name": "signature",
				"type": "bytes"
			}
		]
	},
	{
		"type": "function",
		"name": "Delegate",
		"constant": false,
		"inputs": [
			{
				"name": "candidate",
				"type": "address"
			}
		]
	},
	{
		"type": "function",
		"name": "UnDelegate",
		"constant": false,
		"inputs": [
			{
				"name": "candidate",
				"type": "address"
			},
			{
				"name": "amount",
				"type": "uint256"
			}
		]
	},
	{
		"type": "function",
		"name": "Register",
		"constant": false,
		"inputs": [
			{
				"name": "pubkey",
				"type": "bytes"
			},
            {
				"name": "signature",
				"type": "bytes"
			},
			{
				"name": "commission",
				"type": "uint8"
			}
		]
	},
	{
		"type": "function",
		"name": "UnRegister",
		"constant": false,
		"inputs": []
	},
	{
		"type": "function",
		"name": "SetBlockReward",
		"constant": false,
		"inputs": [
			{
				"name": "chainId",
				"type": "string"
			},
			{
				"name": "reward",
				"type": "uint256"
			}
		]
	},
	{
		"type": "function",
		"name": "EditValidator",
		"constant": false,
		"inputs": [
			{
				"name": "moniker",
				"type": "string"
			},
			{
				"name": "website",
				"type": "string"
			},
			{
				"name": "identity",
				"type": "string"
			},
			{
				"name": "details",
				"type": "string"
			}
		]
	},
	{
		"type": "function",
		"name": "WithdrawReward",
		"constant": false,
		"inputs": [
			{
				"name": "delegateAddress",
				"type": "address"
			}
		]
	},
	{
		"type": "function",
		"name": "UnBanned",
		"constant": false,
		"inputs": []
	},
	{
		"type": "function",
		"name": "SetCommission",
		"constant": false,
		"inputs": [
			{
				"name": "commission",
				"type": "uint8"
			}
		]
	}
]`

var SideChainTokenIncentiveAddr = common.StringToAddress("NEATioSideChainsTokenDepositAddy")

var ChainContractMagicAddr = common.StringToAddress("NEATioMiningSmartContractAddress")

var ChainABI abi.ABI

func init() {
	var err error
	ChainABI, err = abi.JSON(strings.NewReader(jsonChainABI))
	if err != nil {
		panic("fail to create the chain ABI: " + err.Error())
	}
}

func IsNeatChainContractAddr(addr *common.Address) bool {
	return addr != nil && *addr == ChainContractMagicAddr
}

func FunctionTypeFromId(sigdata []byte) (FunctionType, error) {
	m, err := ChainABI.MethodById(sigdata)
	if err != nil {
		return Unknown, err
	}

	return StringToFunctionType(m.Name), nil
}
