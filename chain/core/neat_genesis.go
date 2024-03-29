package core

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/neatio-net/neatio/chain/core/rawdb"
	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/neatdb"
	"github.com/neatio-net/neatio/params"
	"github.com/neatio-net/neatio/utilities/common"
)

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
		Coinbase:   common.HexToAddress(genesisW.Coinbase),
		Alloc:      GenesisAlloc{},
	}

	for k, v := range genesisW.Alloc {
		genesis.Alloc[common.HexToAddress(k)] = v
	}

	return SetupGenesisBlockEx(chainDb, &genesis)
}

func SetupGenesisBlockEx(db neatdb.Database, genesis *Genesis) (*types.Block, error) {

	if genesis != nil && genesis.Config == nil {
		return nil, errGenesisNoConfig
	}

	var block *types.Block = nil
	var err error = nil

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

	if genesis != nil {
		block = genesis.ToBlock(nil)
		hash := block.Hash()
		if hash != stored {
			return nil, &GenesisMismatchError{stored, hash}
		}
	}

	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return block, err
	}

	if genesis == nil && stored != params.MainnetGenesisHash {
		return block, nil
	}

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

func SetupGenesisBlockWithDefault(db neatdb.Database, genesis *Genesis, isMainChain, isTestnet bool) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {

		return nil, common.Hash{}, errGenesisNoConfig
	}

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

		return genesis.Config, block.Hash(), err
	}

	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {

			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}

	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)

		return newcfg, stored, nil
	}

	if genesis == nil && stored != params.MainnetGenesisHash {

		return storedcfg, stored, nil
	}

	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {

		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {

		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)

	return newcfg, stored, nil
}

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
		Coinbase:   common.HexToAddress(genesisW.Coinbase),
		Alloc:      GenesisAlloc{},
	}

	for i, v := range genesisW.Alloc {
		genesis.Alloc[common.HexToAddress(i)] = v
	}

	return &genesis
}

var DefaultMainnetGenesisJSON = `{
	"config": {
			"neatChainId": "neatio",
			"chainId": 1001,
			"homesteadBlock": 0,
			"eip150Block": 0,
			"eip150Hash": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"eip155Block": 0,
			"eip158Block": 0,
			"byzantiumBlock": 0,
			"petersburgBlock": 0,
			"istanbulBlock": 0,
			"neatcon": {
					"epoch": 30000,
					"policy": 0
			}
	},
	"nonce": "0x0",
	"timestamp": "0x654fd36e",
	"extraData": "0x",
	"gasLimit": "0xe0000000",
	"difficulty": "0x1",
	"mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"coinbase": "0x0000000000000000000000000000000000000000",
	"alloc": {
			"c1a8184a86702622c14985c144424f02dede8448": {
					"balance": "0x383f8f62ee6f1ec4000000",
					"amount": "0x152d02c7e14af6800000"
			}
	},
	"number": "0x0",
	"gasUsed": "0x0",
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
}`

var DefaultTestnetGenesisJSON = `{
	"config": {
			"neatChainId": "neatio",
			"chainId": 1001,
			"homesteadBlock": 0,
			"eip150Block": 0,
			"eip150Hash": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"eip155Block": 0,
			"eip158Block": 0,
			"byzantiumBlock": 0,
			"petersburgBlock": 0,
			"istanbulBlock": 0,
			"neatcon": {
					"epoch": 30000,
					"policy": 0
			}
	},
	"nonce": "0x0",
	"timestamp": "0x62579611",
	"extraData": "0x",
	"gasLimit": "0xe0000000",
	"difficulty": "0x1",
	"mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"coinbase": "0x0000000000000000000000000000000000000000",
	"alloc": {
			"a67175cdaf47b91f2aa332d8ec44409a4890f0c4": {
					"balance": "0x31729a3b22ff18d800000",
					"amount": "0xa968163f0a57b400000"
			}
	},
	"number": "0x0",
	"gasUsed": "0x0",
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
}`
