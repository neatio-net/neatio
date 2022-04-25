package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/neatlab/neatio/utilities/common/hexutil"

	"github.com/neatlab/neatio/utilities/common"
	. "github.com/neatlib/common-go"
	"github.com/neatlib/crypto-go"
)

var GenDocKey = []byte("GenDocKey")

var CONSENSUS_NEATCON string = "neatcon"

type GenesisValidator struct {
	EthAccount     common.Address `json:"address"`
	PubKey         crypto.PubKey  `json:"pub_key"`
	Amount         *big.Int       `json:"amount"`
	Name           string         `json:"name"`
	RemainingEpoch uint64         `json:"epoch"`
}

type GenesisCandidate struct {
	EthAccount common.Address `json:"address"`
	PubKey     crypto.PubKey  `json:"pub_key"`
}

type OneEpochDoc struct {
	Number         uint64             `json:"number"`
	RewardPerBlock *big.Int           `json:"reward_per_block"`
	StartBlock     uint64             `json:"start_block"`
	EndBlock       uint64             `json:"end_block"`
	Status         int                `json:"status"`
	Validators     []GenesisValidator `json:"validators"`
}

type RewardSchemeDoc struct {
	TotalReward        *big.Int `json:"total_reward"`
	RewardFirstYear    *big.Int `json:"reward_first_year"`
	EpochNumberPerYear uint64   `json:"epoch_no_per_year"`
	TotalMintingYears  uint64   `json:"total_year"`
}

type GenesisDoc struct {
	ChainID      string          `json:"chain_id"`
	Consensus    string          `json:"consensus"`
	GenesisTime  time.Time       `json:"genesis_time"`
	RewardScheme RewardSchemeDoc `json:"reward_scheme"`
	CurrentEpoch OneEpochDoc     `json:"current_epoch"`
}

func (genDoc *GenesisDoc) SaveAs(file string) error {
	genDocBytes, err := json.MarshalIndent(genDoc, "", "\t")
	if err != nil {
		fmt.Println(err)
	}

	return WriteFile(file, genDocBytes, 0644)
}

func GenesisDocFromJSON(jsonBlob []byte) (genDoc *GenesisDoc, err error) {
	err = json.Unmarshal(jsonBlob, &genDoc)
	return
}

var MainnetGenesisJSON string = `{
	"chain_id": "neatio",
	"consensus": "neatcon",
	"genesis_time": "2022-04-24T05:19:13.379479375Z",
	"reward_scheme": {
			"total_reward": "0x24c92e57292290bcd00000",
			"reward_first_year": "0x273cfe3ad68b450daaaaa",
			"epoch_no_per_year": "0x2238",
			"total_year": "0xf"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x1fbf447a582cdf8",
			"start_block": "0x0",
			"end_block": "0x960",
			"validators": [
					{
							"address": "0x03ba7541d4484155c7d08b398d6ade9f34bd8363",
							"pub_key": "0x5432CA8623B13E919C04D3959D2D7C43DB5AAAF342F89AA66021D05BF0E45DA26B7ADB7040FC04BD80130415696ED2FB6BC4A6C15D542877AC9EEAAA945F6BF470A43CC9A42775B1790D5386CA50380D3B92B5E70F189C402306682DCA6739C215695F4BC4481E2A8B56DF4AB136BBB70A4141F119C9B305A185D2DBCDAEDDFF",
							"amount": "0xa968163f0a57b400000",
							"name": "",
							"epoch": "0x0"
					}
			]
	}
}`

var TestnetGenesisJSON string = `{
	"chain_id": "neatio",
	"consensus": "neatcon",
	"genesis_time": "2022-04-14T03:33:46.520459207Z",
	"reward_scheme": {
			"total_reward": "0x2644bfbcd11fc43d1c0000",
			"reward_first_year": "0x28d1dd96346626a795555",
			"epoch_no_per_year": "0x2238",
			"total_year": "0xf"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x160490190ea702c",
			"start_block": "0x0",
			"end_block": "0xe10",
			"validators": [
					{
							"address": "0xa67175CDAf47B91f2Aa332d8eC44409A4890f0c4",
							"pub_key": "0x28549322396163D2B1B1F160D40953A2D166D9736C9672B3C3D39CC3123D262B50261C498CE8251FAE63A5CC683591C6707ECE866A699D2A3D6C51D3D902F5A350041DFA482F746D459EB15F4665B6808953A22A5C5A5473CF9EB69D822A3B8260CB0EA41A543DEB0F5EB61F705C68DC3496104EFCEEA8A97DC3A4023EB9B302",
							"amount": "0xa968163f0a57b400000",
							"name": "",
							"epoch": "0x0"
					}
			]
	}
}`

func (ep OneEpochDoc) MarshalJSON() ([]byte, error) {
	type hexEpoch struct {
		Number         hexutil.Uint64     `json:"number"`
		RewardPerBlock *hexutil.Big       `json:"reward_per_block"`
		StartBlock     hexutil.Uint64     `json:"start_block"`
		EndBlock       hexutil.Uint64     `json:"end_block"`
		Validators     []GenesisValidator `json:"validators"`
	}
	var enc hexEpoch
	enc.Number = hexutil.Uint64(ep.Number)
	enc.RewardPerBlock = (*hexutil.Big)(ep.RewardPerBlock)
	enc.StartBlock = hexutil.Uint64(ep.StartBlock)
	enc.EndBlock = hexutil.Uint64(ep.EndBlock)
	if ep.Validators != nil {
		enc.Validators = ep.Validators
	}
	return json.Marshal(&enc)
}

func (ep *OneEpochDoc) UnmarshalJSON(input []byte) error {
	type hexEpoch struct {
		Number         hexutil.Uint64     `json:"number"`
		RewardPerBlock *hexutil.Big       `json:"reward_per_block"`
		StartBlock     hexutil.Uint64     `json:"start_block"`
		EndBlock       hexutil.Uint64     `json:"end_block"`
		Validators     []GenesisValidator `json:"validators"`
	}
	var dec hexEpoch
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	ep.Number = uint64(dec.Number)
	ep.RewardPerBlock = (*big.Int)(dec.RewardPerBlock)
	ep.StartBlock = uint64(dec.StartBlock)
	ep.EndBlock = uint64(dec.EndBlock)
	if dec.Validators == nil {
		return errors.New("missing required field 'validators' for Genesis/epoch")
	}
	ep.Validators = dec.Validators
	return nil
}

func (gv GenesisValidator) MarshalJSON() ([]byte, error) {
	type hexValidator struct {
		Address        common.Address `json:"address"`
		PubKey         string         `json:"pub_key"`
		Amount         *hexutil.Big   `json:"amount"`
		Name           string         `json:"name"`
		RemainingEpoch hexutil.Uint64 `json:"epoch"`
	}
	var enc hexValidator
	enc.Address = gv.EthAccount
	enc.PubKey = gv.PubKey.KeyString()
	enc.Amount = (*hexutil.Big)(gv.Amount)
	enc.Name = gv.Name
	enc.RemainingEpoch = hexutil.Uint64(gv.RemainingEpoch)

	return json.Marshal(&enc)
}

func (gv *GenesisValidator) UnmarshalJSON(input []byte) error {
	type hexValidator struct {
		Address        common.Address `json:"address"`
		PubKey         string         `json:"pub_key"`
		Amount         *hexutil.Big   `json:"amount"`
		Name           string         `json:"name"`
		RemainingEpoch hexutil.Uint64 `json:"epoch"`
	}
	var dec hexValidator
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	gv.EthAccount = dec.Address

	pubkeyBytes := common.FromHex(dec.PubKey)
	if dec.PubKey == "" || len(pubkeyBytes) != 128 {
		return errors.New("wrong format of required field 'pub_key' for Genesis/epoch/validators")
	}
	var blsPK crypto.BLSPubKey
	copy(blsPK[:], pubkeyBytes)
	gv.PubKey = blsPK

	if dec.Amount == nil {
		return errors.New("missing required field 'amount' for Genesis/epoch/validators")
	}
	gv.Amount = (*big.Int)(dec.Amount)
	gv.Name = dec.Name
	gv.RemainingEpoch = uint64(dec.RemainingEpoch)
	return nil
}

func (rs RewardSchemeDoc) MarshalJSON() ([]byte, error) {
	type hexRewardScheme struct {
		TotalReward        *hexutil.Big   `json:"total_reward"`
		RewardFirstYear    *hexutil.Big   `json:"reward_first_year"`
		EpochNumberPerYear hexutil.Uint64 `json:"epoch_no_per_year"`
		TotalMintingYears  hexutil.Uint64 `json:"total_year"`
	}
	var enc hexRewardScheme
	enc.TotalReward = (*hexutil.Big)(rs.TotalReward)
	enc.RewardFirstYear = (*hexutil.Big)(rs.RewardFirstYear)
	enc.EpochNumberPerYear = hexutil.Uint64(rs.EpochNumberPerYear)
	enc.TotalMintingYears = hexutil.Uint64(rs.TotalMintingYears)

	return json.Marshal(&enc)
}

func (rs *RewardSchemeDoc) UnmarshalJSON(input []byte) error {
	type hexRewardScheme struct {
		TotalReward        *hexutil.Big   `json:"total_reward"`
		RewardFirstYear    *hexutil.Big   `json:"reward_first_year"`
		EpochNumberPerYear hexutil.Uint64 `json:"epoch_no_per_year"`
		TotalMintingYears  hexutil.Uint64 `json:"total_year"`
	}
	var dec hexRewardScheme
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.TotalReward == nil {
		return errors.New("missing required field 'total_reward' for Genesis/reward_scheme")
	}
	rs.TotalReward = (*big.Int)(dec.TotalReward)
	if dec.RewardFirstYear == nil {
		return errors.New("missing required field 'reward_first_year' for Genesis/reward_scheme")
	}
	rs.RewardFirstYear = (*big.Int)(dec.RewardFirstYear)

	rs.EpochNumberPerYear = uint64(dec.EpochNumberPerYear)
	rs.TotalMintingYears = uint64(dec.TotalMintingYears)

	return nil
}
