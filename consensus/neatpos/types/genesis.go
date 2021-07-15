package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/neatlab/neatio/common/hexutil"

	"github.com/neatlab/neatio/common"
	. "github.com/neatlib/common-go"
	"github.com/neatlib/crypto-go"
)

//------------------------------------------------------------
// we store the gendoc in the db

var GenDocKey = []byte("GenDocKey")

//------------------------------------------------------------
// core types for a genesis definition

var CONSENSUS_POS string = "pos"
var CONSENSUS_POW string = "pow"
var Consensus_NeatPoS string = "neatpos"

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
	Consensus    string          `json:"consensus"` //should be 'pos' or 'pow'
	GenesisTime  time.Time       `json:"genesis_time"`
	RewardScheme RewardSchemeDoc `json:"reward_scheme"`
	CurrentEpoch OneEpochDoc     `json:"current_epoch"`
}

// Write to file use
type GenesisDocWrite struct {
	ChainID      string           `json:"chain_id"`
	Consensus    string           `json:"consensus"` //should be 'pos' or 'pow'
	GenesisTime  time.Time        `json:"genesis_time"`
	RewardScheme RewardSchemeDoc  `json:"reward_scheme"`
	CurrentEpoch OneEpochDocWrite `json:"current_epoch"`
}

// Write to file use
type OneEpochDocWrite struct {
	Number         uint64                  `json:"number"`
	RewardPerBlock *big.Int                `json:"reward_per_block"`
	StartBlock     uint64                  `json:"start_block"`
	EndBlock       uint64                  `json:"end_block"`
	Status         int                     `json:"status"`
	Validators     []GenesisValidatorWrite `json:"validators"`
}

// Write to file use
type GenesisValidatorWrite struct {
	EthAccount     string        `json:"address"`
	PubKey         crypto.PubKey `json:"pub_key"`
	Amount         *big.Int      `json:"amount"`
	Name           string        `json:"name"`
	RemainingEpoch uint64        `json:"epoch"`
}

// Utility method for saving GenensisDoc as JSON file.
//func (genDoc *GenesisDoc) SaveAs(file string) error {
//	genDocBytes, err := json.MarshalIndent(genDoc, "", "\t")
//	if err != nil {
//		fmt.Println(err)
//	}
//
//	return WriteFile(file, genDocBytes, 0644)
//}
// Write to file use
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

//------------------------------------------------------------
// Make genesis state from file

//func GenesisDocFromJSON(jsonBlob []byte) (genDoc *GenesisDoc, err error) {
//	err = json.Unmarshal(jsonBlob, &genDoc)
//	return
//}

// Read genesisdocjson and do the conversion
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
	"consensus": "neatpos",
	"genesis_time": "2021-07-15T17:23:46.552259904+02:00",
	"reward_scheme": {
			"total_reward": "0x98f2ea83765ac3cf90200000",
			"reward_first_year": "0x51929f350bec242a6f00000",
			"epoch_no_per_year": "0x88f",
			"total_year": "0x1e"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x2b5e3af16b1880000",
			"start_block": "0x0",
			"end_block": "0x3840",
			"validators": [
					{
							"address": "NEATh2XbRB3F26NVGTSD36AFzdXHUm7s",
							"pub_key": "0x84CA5708F07EB2DDC947483A487B1FEC9FB346814C9094510587CC4CE3BE88FB52BC4DD4561813E991B50857A53F6963CCB59CCDF617EFFD1D79D16AA6D2DDD76E3F304E9BC805483F7180EEFA42D8A4DFC17A7FC619DE8E9AFBC85C372C5E28104B748D6FFE139D5C8A9D0FB7028DDB644AEAEFDCBEA2C80AA8C244348EAFAD"
							"amount": "0x422ca8b0a00a425000000",
							"name": "",
							"epoch": "0x0"
					}
			]
	}
}`

var TestnetGenesisJSON string = `{
	"chain_id": "testnet",
	"consensus": "neatpos",
	"genesis_time": "2021-04-16T19:51:48.738131345+02:00",
	"reward_scheme": {
			"total_reward": "0x98f2ea83765ac3cf90200000",
			"reward_first_year": "0x51929f350bec242a6f00000",
			"epoch_no_per_year": "0x88f",
			"total_year": "0x1e"
	},
	"current_epoch": {
			"number": "0x0",
			"reward_per_block": "0x2b5e3af16b1880000",
			"start_block": "0x0",
			"end_block": "0x3840",
			"validators": [
					{
							"address": "NEATjAfa8qqoZC9zXvHesXnjEfTFgm8q",
							"pub_key": "0x10219662B558A41671120889479E08C0826E542B37B9F6DD9CAE0ACF35C7FDF841EB688E8C2E4244ECB712B30B5F6C18AFFA35897A829C3589562BC990AC0A08318CD6F63476A368183CDA34EFD3F13D6ABF2A59C187B6367B904AE55DA481E85AC62D460FA7E26D76A792CBBC1C1E52F85D93A736C33B21909A4CA62A54B8DC",
							"amount": "0x3635c9adc5dea00000",
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

// Write to the intermediate conversion of the file
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

// Write to the intermediate conversion of the file
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

// Write to the intermediate conversion of the file
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

// Write to the intermediate conversion of the file
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
