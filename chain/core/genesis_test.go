package core

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/neatlab/neatio/chain/core/rawdb"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
)

func TestDefaultGenesisBlock(t *testing.T) {
	block := DefaultGenesisBlock().ToBlock(nil)
	if block.Hash() != params.MainnetGenesisHash {
		t.Errorf("wrong mainnet genesis hash, got %v, want %v", block.Hash(), params.MainnetGenesisHash)
	}
	block = DefaultTestnetGenesisBlock().ToBlock(nil)
	if block.Hash() != params.TestnetGenesisHash {
		t.Errorf("wrong testnet genesis hash, got %v, want %v", block.Hash(), params.TestnetGenesisHash)
	}
}

func TestSetupGenesis(t *testing.T) {
	var (
		customghash = common.HexToHash("0x89c99d90b79719238d2645c7642f2c9295246e80775b38cfd162b696817fbd50")
		customg     = Genesis{
			Config: &params.ChainConfig{HomesteadBlock: big.NewInt(3)},
			Alloc: GenesisAlloc{
				{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			},
		}
		oldcustomg = customg
	)
	oldcustomg.Config = &params.ChainConfig{HomesteadBlock: big.NewInt(2)}
	tests := []struct {
		name       string
		fn         func(neatdb.Database) (*params.ChainConfig, common.Hash, error)
		wantConfig *params.ChainConfig
		wantHash   common.Hash
		wantErr    error
	}{
		{
			name: "genesis without ChainConfig",
			fn: func(db neatdb.Database) (*params.ChainConfig, common.Hash, error) {
				return SetupGenesisBlock(db, new(Genesis))
			},
			wantErr:    errGenesisNoConfig,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "no block in DB, genesis == nil",
			fn: func(db neatdb.Database) (*params.ChainConfig, common.Hash, error) {
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db neatdb.Database) (*params.ChainConfig, common.Hash, error) {
				DefaultGenesisBlock().MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "custom block in DB, genesis == nil",
			fn: func(db neatdb.Database) (*params.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "custom block in DB, genesis == testnet",
			fn: func(db neatdb.Database) (*params.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, DefaultTestnetGenesisBlock())
			},
			wantErr:    &GenesisMismatchError{Stored: customghash, New: params.TestnetGenesisHash},
			wantHash:   params.TestnetGenesisHash,
			wantConfig: params.TestnetChainConfig,
		},
		{
			name: "compatible config in DB",
			fn: func(db neatdb.Database) (*params.ChainConfig, common.Hash, error) {
				oldcustomg.MustCommit(db)
				return SetupGenesisBlock(db, &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "incompatible config in DB",

			wantHash:   customghash,
			wantConfig: customg.Config,
			wantErr: &params.ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(2),
				NewConfig:    big.NewInt(3),
				RewindTo:     1,
			},
		},
	}

	for _, test := range tests {
		db := rawdb.NewMemoryDatabase()
		config, hash, err := test.fn(db)

		if !reflect.DeepEqual(err, test.wantErr) {
			spew := spew.ConfigState{}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spew.NewFormatter(err), spew.NewFormatter(test.wantErr))
		}
		if !reflect.DeepEqual(config, test.wantConfig) {
			t.Errorf("%s:\nreturned %v\nwant     %v", test.name, config, test.wantConfig)
		}
		if hash != test.wantHash {
			t.Errorf("%s: returned hash %s, want %s", test.name, hash.Hex(), test.wantHash.Hex())
		} else if err == nil {

			stored := rawdb.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}
