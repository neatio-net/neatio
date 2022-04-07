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
	TotalMintingYears  uint64   `json:"total_year"`
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
			EthAccount:     common.HexToAddress(v.EthAccount),
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
	"genesis_time": "2022-04-05T19:50:26.316057065Z",
	"reward_scheme": {
			"total_reward": "0x2644bfbcd11fc43d1c0000",
			"reward_first_year": "0x28d1dd96346626a795555",
			"epoch_no_per_year": "0x16d",
			"total_year": "0xf"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x160490190ea702c",
			"start_block": "0x0",
			"end_block": "0x15180",
			"validators": [
					{
							"address": "0x58cE08A1227B7Fd1d5B4B626B29C96f31aB3B4D4",
							"pub_key": "0x22CAFC699E9908AE852E5DC3A2A5B0689451080EA57B2C350E6F58E9304D25F1332C6C4B3C80C85FFE142019D5AE645A146190E5B0F5B6DCC271654CF627E66B457BFC639059C226A6B6A162F7653CABBC401515BBCCAA64EDE03B888DECFA4E2A640B1EEE3600AB179EA5432B0EBCB3B8043399DFD81C55AF9A35868E200B99",
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
	"genesis_time": "2022-04-05T19:50:26.316057065Z",
	"reward_scheme": {
			"total_reward": "0x2644bfbcd11fc43d1c0000",
			"reward_first_year": "0x28d1dd96346626a795555",
			"epoch_no_per_year": "0x16d",
			"total_year": "0xf"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x160490190ea702c",
			"start_block": "0x0",
			"end_block": "0x15180",
			"validators": [
					{
							"address": "0x58cE08A1227B7Fd1d5B4B626B29C96f31aB3B4D4",
							"pub_key": "0x22CAFC699E9908AE852E5DC3A2A5B0689451080EA57B2C350E6F58E9304D25F1332C6C4B3C80C85FFE142019D5AE645A146190E5B0F5B6DCC271654CF627E66B457BFC639059C226A6B6A162F7653CABBC401515BBCCAA64EDE03B888DECFA4E2A640B1EEE3600AB179EA5432B0EBCB3B8043399DFD81C55AF9A35868E200B99",
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
