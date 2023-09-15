package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/nio-net/nio/utilities/common/hexutil"

	. "github.com/nio-net/common"
	"github.com/nio-net/crypto"
	"github.com/nio-net/nio/utilities/common"
)

var GenDocKey = []byte("GenDocKey")

var CONSENSUS_POS string = "pos"
var CONSENSUS_POW string = "pow"
var CONSENSUS_NeatCon string = "neatcon"

type GenesisValidator struct {
	EthAccount     common.Address `json:"address"`
	PubKey         crypto.PubKey  `json:"pub_key"`
	Amount         *big.Int       `json:"amount"`
	Name           string         `json:"name"`
	RemainingEpoch uint64         `json:"epoch"`
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
	TotalYear          uint64   `json:"total_year"`
}

type GenesisDoc struct {
	ChainID      string          `json:"chain_id"`
	Consensus    string          `json:"consensus"`
	GenesisTime  time.Time       `json:"genesis_time"`
	RewardScheme RewardSchemeDoc `json:"reward_scheme"`
	CurrentEpoch OneEpochDoc     `json:"current_epoch"`
}

type GenesisDocWrite struct {
	ChainID      string           `json:"chain_id"`
	Consensus    string           `json:"consensus"`
	GenesisTime  time.Time        `json:"genesis_time"`
	RewardScheme RewardSchemeDoc  `json:"reward_scheme"`
	CurrentEpoch OneEpochDocWrite `json:"current_epoch"`
}

type OneEpochDocWrite struct {
	Number         uint64                  `json:"number"`
	RewardPerBlock *big.Int                `json:"reward_per_block"`
	StartBlock     uint64                  `json:"start_block"`
	EndBlock       uint64                  `json:"end_block"`
	Status         int                     `json:"status"`
	Validators     []GenesisValidatorWrite `json:"validators"`
}

type GenesisValidatorWrite struct {
	EthAccount     string        `json:"address"`
	PubKey         crypto.PubKey `json:"pub_key"`
	Amount         *big.Int      `json:"amount"`
	Name           string        `json:"name"`
	RemainingEpoch uint64        `json:"epoch"`
}

func (genDoc *GenesisDoc) SaveAs(file string) error {

	genDocWrite := GenesisDocWrite{
		ChainID:      genDoc.ChainID,
		Consensus:    genDoc.Consensus,
		GenesisTime:  genDoc.GenesisTime,
		RewardScheme: genDoc.RewardScheme,
		CurrentEpoch: OneEpochDocWrite{
			Number:         genDoc.CurrentEpoch.Number,
			RewardPerBlock: genDoc.CurrentEpoch.RewardPerBlock,
			StartBlock:     genDoc.CurrentEpoch.StartBlock,
			EndBlock:       genDoc.CurrentEpoch.EndBlock,
			Status:         genDoc.CurrentEpoch.Status,
			Validators:     make([]GenesisValidatorWrite, len(genDoc.CurrentEpoch.Validators)),
		},
	}
	for i, v := range genDoc.CurrentEpoch.Validators {
		genDocWrite.CurrentEpoch.Validators[i] = GenesisValidatorWrite{
			EthAccount:     v.EthAccount.String(),
			PubKey:         v.PubKey,
			Amount:         v.Amount,
			Name:           v.Name,
			RemainingEpoch: v.RemainingEpoch,
		}
	}

	genDocBytes, err := json.MarshalIndent(genDocWrite, "", "\t")
	if err != nil {
		fmt.Println(err)
	}

	return WriteFile(file, genDocBytes, 0644)
}

func GenesisDocFromJSON(jsonBlob []byte) (genDoc *GenesisDoc, err error) {
	var genDocWrite *GenesisDocWrite
	err = json.Unmarshal(jsonBlob, &genDocWrite)
	if err != nil {
		return &GenesisDoc{}, err
	}

	genDoc = &GenesisDoc{
		ChainID:      genDocWrite.ChainID,
		Consensus:    genDocWrite.Consensus,
		GenesisTime:  genDocWrite.GenesisTime,
		RewardScheme: genDocWrite.RewardScheme,
		CurrentEpoch: OneEpochDoc{
			Number:         genDocWrite.CurrentEpoch.Number,
			RewardPerBlock: genDocWrite.CurrentEpoch.RewardPerBlock,
			StartBlock:     genDocWrite.CurrentEpoch.StartBlock,
			EndBlock:       genDocWrite.CurrentEpoch.EndBlock,
			Status:         genDocWrite.CurrentEpoch.Status,
			Validators:     make([]GenesisValidator, len(genDocWrite.CurrentEpoch.Validators)),
		},
	}
	for i, v := range genDocWrite.CurrentEpoch.Validators {
		genDoc.CurrentEpoch.Validators[i] = GenesisValidator{
			EthAccount:     common.StringToAddress(v.EthAccount),
			PubKey:         v.PubKey,
			Amount:         v.Amount,
			Name:           v.Name,
			RemainingEpoch: v.RemainingEpoch,
		}
	}

	return
}

var MainnetGenesisJSON string = `{
	"chain_id": "neatio",
	"consensus": "neatcon",
	"genesis_time": "2022-01-01T01:00:03.947586096+01:00",
	"reward_scheme": {
			"total_reward": "0x3bc350d642877320400000",
			"reward_first_year": "0x20f90076365c62d400000",
			"epoch_no_per_year": "0x16d",
			"total_year": "0x1d"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x1d0cae4f5dd000",
			"start_block": "0x0",
			"end_block": "0x151b9",
			"validators": [
					{
							"address": "NEATxdFbo7zsqQqr829U9p7rFja1ZGyk",
							"pub_key": "0x1F100176AA1DA63BEDD72FA0463162DDB78D9E90AAB36A6E90B6CA1B7196C29F0CF878BCB9DDD78135BD551891B5B8D3B248264BB72DE7F64D17EEC6EE091E9958B0B9C5C3646D>
							"amount": "0x104e2da94483f6200000",
							"name": "",
							"epoch": "0x0"
					}
			]
	}
}`

var TestnetGenesisJSON string = `{
	"chain_id": "neatio",
	"consensus": "neatcon",
	"genesis_time": "2021-08-29T08:22:48.630594977+02:00",
	"reward_scheme": {
			"total_reward": "0x3bc350d642877320400000",
			"reward_first_year": "0x20f90076365c62d400000",
			"epoch_no_per_year": "0x2238",
			"total_year": "0x1d"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x1c110215b9c000",
			"start_block": "0x0",
			"end_block": "0xe10",
			"validators": [
					{
							"address": "NEATvs9MgomEMzya1RFGuJxUhsJbujKy",
							"pub_key": "0x1D39A5FBF22E6EE55595F3021A6846F250C42A48E98AC21F0BB584FA62AE5A936CFAF51F97A406FC7EAF74C8640B4B3E2E041E99D0A5736E7E6B5D8F0D54992000FF1329824700AAAB01D616D43943007F3E4A357F910EC2DB3E15112ADEBBE07EF3E95C7641C249AAB335E37515F5FD9F818F915A48F7B56FE3F5702392EBB5",
							"amount": "0x104e2da94483f6200000",
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

func (ep OneEpochDocWrite) MarshalJSON() ([]byte, error) {
	type hexEpoch struct {
		Number         hexutil.Uint64          `json:"number"`
		RewardPerBlock *hexutil.Big            `json:"reward_per_block"`
		StartBlock     hexutil.Uint64          `json:"start_block"`
		EndBlock       hexutil.Uint64          `json:"end_block"`
		Validators     []GenesisValidatorWrite `json:"validators"`
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

func (ep *OneEpochDocWrite) UnmarshalJSON(input []byte) error {
	type hexEpoch struct {
		Number         hexutil.Uint64          `json:"number"`
		RewardPerBlock *hexutil.Big            `json:"reward_per_block"`
		StartBlock     hexutil.Uint64          `json:"start_block"`
		EndBlock       hexutil.Uint64          `json:"end_block"`
		Validators     []GenesisValidatorWrite `json:"validators"`
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

func (gv GenesisValidatorWrite) MarshalJSON() ([]byte, error) {
	type hexValidator struct {
		Address        string         `json:"address"`
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

func (gv *GenesisValidatorWrite) UnmarshalJSON(input []byte) error {
	type hexValidator struct {
		Address        string         `json:"address"`
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
		TotalYear          hexutil.Uint64 `json:"total_year"`
	}
	var enc hexRewardScheme
	enc.TotalReward = (*hexutil.Big)(rs.TotalReward)
	enc.RewardFirstYear = (*hexutil.Big)(rs.RewardFirstYear)
	enc.EpochNumberPerYear = hexutil.Uint64(rs.EpochNumberPerYear)
	enc.TotalYear = hexutil.Uint64(rs.TotalYear)

	return json.Marshal(&enc)
}

func (rs *RewardSchemeDoc) UnmarshalJSON(input []byte) error {
	type hexRewardScheme struct {
		TotalReward        *hexutil.Big   `json:"total_reward"`
		RewardFirstYear    *hexutil.Big   `json:"reward_first_year"`
		EpochNumberPerYear hexutil.Uint64 `json:"epoch_no_per_year"`
		TotalYear          hexutil.Uint64 `json:"total_year"`
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
	rs.TotalYear = uint64(dec.TotalYear)

	return nil
}
