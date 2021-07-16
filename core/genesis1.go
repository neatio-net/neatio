package core

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/core/rawdb"
	"github.com/neatlab/neatio/core/types"
	"github.com/neatlab/neatio/log"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/params"
)

// WriteGenesisBlock writes the genesis block to the database as block number 0
//func WriteGenesisBlock(chainDb neatdb.Database, reader io.Reader) (*types.Block, error) {
//	contents, err := ioutil.ReadAll(reader)
//	if err != nil {
//		return nil, err
//	}
//
//	var genesis = Genesis{}
//
//	if err := json.Unmarshal(contents, &genesis); err != nil {
//		return nil, err
//	}
//
//	return SetupGenesisBlockEx(chainDb, &genesis)
//}

func WriteGenesisBlock(chainDb neatdb.Database, reader io.Reader) (*types.Block, error) {
	contents, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var (
		genesis  Genesis
		genesisW GenesisWrite
	)

	genesis.Alloc = GenesisAlloc{}

	if err := json.Unmarshal(contents, &genesisW); err != nil {
		return nil, err
	}

	genesis = Genesis{
		Config:     genesisW.Config,
		Nonce:      genesisW.Nonce,
		Timestamp:  genesisW.Timestamp,
		ParentHash: genesisW.ParentHash,
		ExtraData:  genesisW.ExtraData,
		GasLimit:   genesisW.GasLimit,
		Difficulty: genesisW.Difficulty,
		Mixhash:    genesisW.Mixhash,
		Coinbase:   common.StringToAddress(genesisW.Coinbase),
		Alloc:      GenesisAlloc{},
	}

	for k, v := range genesisW.Alloc {
		genesis.Alloc[common.StringToAddress(k)] = v
	}

	return SetupGenesisBlockEx(chainDb, &genesis)
}

func SetupGenesisBlockEx(db neatdb.Database, genesis *Genesis) (*types.Block, error) {

	if genesis != nil && genesis.Config == nil {
		return nil, errGenesisNoConfig
	}

	var block *types.Block = nil
	var err error = nil

	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err = genesis.Commit(db)
		return block, err
	}

	// Check whether the genesis block is already written.
	if genesis != nil {
		block = genesis.ToBlock(nil)
		hash := block.Hash()
		if hash != stored {
			return nil, &GenesisMismatchError{stored, hash}
		}
	}

	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return block, err
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != params.MainnetGenesisHash {
		return block, nil
	}

	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return nil, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return nil, compatErr
	}
	return block, err
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlockWithDefault(db neatdb.Database, genesis *Genesis, isMainChain, isTestnet bool) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 1\n")
		return nil, common.Hash{}, errGenesisNoConfig
	}

	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{} && isMainChain) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			if isTestnet {
				genesis = DefaultGenesisBlockFromJson(DefaultTestnetGenesisJSON)
			} else {
				genesis = DefaultGenesisBlockFromJson(DefaultMainnetGenesisJSON)
			}
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 2\n")
		return genesis.Config, block.Hash(), err
	}

	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 3\n")
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}

	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 4\n")
		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != params.MainnetGenesisHash {
		//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 5\n")
		return storedcfg, stored, nil
	}

	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 6\n")
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 7\n")
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	//fmt.Printf("core genesis1 SetupGenesisBlockWithDefault 8\n")
	return newcfg, stored, nil
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
func DefaultGenesisBlockFromJson(genesisJson string) *Genesis {

	var (
		genesis  Genesis
		genesisW GenesisWrite
	)

	genesis.Alloc = GenesisAlloc{}
	if err := json.Unmarshal([]byte(genesisJson), &genesisW); err != nil {
		return nil
	}

	genesis = Genesis{
		Config:     genesisW.Config,
		Nonce:      genesisW.Nonce,
		Timestamp:  genesisW.Timestamp,
		ParentHash: genesisW.ParentHash,
		ExtraData:  genesisW.ExtraData,
		GasLimit:   genesisW.GasLimit,
		Difficulty: genesisW.Difficulty,
		Mixhash:    genesisW.Mixhash,
		Coinbase:   common.StringToAddress(genesisW.Coinbase),
		Alloc:      GenesisAlloc{},
	}

	for i, v := range genesisW.Alloc {
		genesis.Alloc[common.StringToAddress(i)] = v
	}

	return &genesis
}

var DefaultMainnetGenesisJSON = `{
	"config": {
			"neatChainId": "neatio",
			"chainId": 1,
			"homesteadBlock": 0,
			"eip150Block": 0,
			"eip150Hash": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"eip155Block": 0,
			"eip158Block": 0,
			"byzantiumBlock": 0,
			"neatpos": {
					"epoch": 65730,
					"policy": 0
			}
	},
	"nonce": 16045690984833335023,
	"timestamp": 1626378762,
	"extraData": "",
	"gasLimit": 100000000,
	"difficulty": 1,
	"mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"coinbase": "NEATAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"alloc": {
			"NEATiuCnJ9pt253LsTJTW7fcw3Ms5vn1": {
					"balance": "0xdc69552cd63a6893300000",
					"amount": "0x422ca8b0a00a425000000"
			}
	},
	"number": 0,
	"gasUsed": 0,
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
}`

var DefaultTestnetGenesisJSON = `{
	"config": {
			"neatChainId": "testnet",
			"chainId": 2,
			"homesteadBlock": 0,
			"eip150Block": 0,
			"eip150Hash": "0x2086799aeebeae135c246c65021c82b4e15a2c451340993aacfd2751886514f0",
			"eip155Block": 0,
			"eip158Block": 0,
			"byzantiumBlock": 0,
			"neatpos": {
					"epoch": 65730,
					"policy": 0
			}
	},
	"nonce": 16045690984833335023,
	"timestamp": 1618595503,
	"extraData": "",
	"gasLimit": 200000000,
	"difficulty": 1,
	"mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"coinbase": "NEATAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"alloc": {
			"NEATjAfa8qqoZC9zXvHesXnjEfTFgm8q": {
					"balance": "0xa18f07d736b90be550000000",
					"amount": "0x3635c9adc5dea00000"
			}
	},
	"number": 0,
	"gasUsed": 0,
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
}`
