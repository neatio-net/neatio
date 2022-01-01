package core

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/neatlab/neatio/chain/core/rawdb"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
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
			"neatcon": {
					"epoch": 86457,
					"policy": 0
			}
	},
	"nonce": "0xdeadbeefdeadbeef",
	"timestamp": "0x61cf9982",
	"extraData": "0x",
	"gasLimit": "0x7270e00",
	"difficulty": "0x1",
	"mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"coinbase": "NEATioBlockchainsGenesisCoinbase",
	"alloc": {
			"NEATxdFbo7zsqQqr829U9p7rFja1ZGyk": {
					"balance": "0x3ee1186f11c064cc00000",
					"amount": "0x104e2da94483f6200000"
			}
	},
	"number": "0x0",
	"gasUsed": "0x0",
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
}`

var DefaultTestnetGenesisJSON = `{
	"config": {
			"neatChainId": "neatio",
			"chainId": 1,
			"homesteadBlock": 0,
			"eip150Block": 0,
			"eip150Hash": "0x0000000000000000000000000000000000000000000000000000000000000000",
			"eip155Block": 0,
			"eip158Block": 0,
			"byzantiumBlock": 0,
			"neatcon": {
					"epoch": 30000,
					"policy": 0
			}
	},
	"nonce": "0xdeadbeefdeadbeef",
	"timestamp": "0x6127cacf",
	"extraData": "0x",
	"gasLimit": "0x7270e00",
	"difficulty": "0x1",
	"mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
	"coinbase": "NEATioBlockchainsGenesisCoinbase",
	"alloc": {
			"NEATjQoTmRNypoWaTpMebKmED8bbA32b": {
					"balance": "0x3ee1186f11c064cc00000",
					"amount": "0x104e2da94483f6200000"
			}
	},
	"number": "0x0",
	"gasUsed": "0x0",
	"parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000"
}`
