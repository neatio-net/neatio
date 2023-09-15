package main

import (
	"path/filepath"

	cfg "github.com/nio-net/config"
	"github.com/nio-net/nio/chain/accounts/keystore"
	ntcTypes "github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/log"
	neatnode "github.com/nio-net/nio/network/node"
	"github.com/nio-net/nio/utilities/utils"
	"gopkg.in/urfave/cli.v1"
)

const (
	MainChain    = "neatio"
	TestnetChain = "testnet"
)

type Chain struct {
	Id       string
	Config   cfg.Config
	NeatNode *neatnode.Node
}

func LoadMainChain(ctx *cli.Context, chainId string) *Chain {

	chain := &Chain{Id: chainId}
	config := utils.GetNeatConConfig(chainId, ctx)
	chain.Config = config

	log.Info("Starting Neatio full node...")
	stack := makeFullNode(ctx, GetCMInstance(ctx).cch, chainId)
	chain.NeatNode = stack

	return chain
}

func LoadSideChain(ctx *cli.Context, chainId string) *Chain {

	log.Infof("now load side: %s", chainId)

	chain := &Chain{Id: chainId}
	config := utils.GetNeatConConfig(chainId, ctx)
	chain.Config = config

	log.Infof("chainId: %s, makeFullNode", chainId)
	cch := GetCMInstance(ctx).cch
	stack := makeFullNode(ctx, cch, chainId)
	if stack == nil {
		return nil
	} else {
		chain.NeatNode = stack
		return chain
	}
}

func StartChain(ctx *cli.Context, chain *Chain, startDone chan<- struct{}) error {

	go func() {
		utils.StartNode(ctx, chain.NeatNode)

		if startDone != nil {
			startDone <- struct{}{}
		}
	}()

	return nil
}

func CreateSideChain(ctx *cli.Context, chainId string, validator ntcTypes.PrivValidator, keyJson []byte, validators []ntcTypes.GenesisValidator) error {

	config := utils.GetNeatConConfig(chainId, ctx)

	if len(keyJson) > 0 {
		keystoreDir := config.GetString("keystore")
		keyJsonFilePath := filepath.Join(keystoreDir, keystore.KeyFileName(validator.Address))
		saveKeyError := keystore.WriteKeyStore(keyJsonFilePath, keyJson)
		if saveKeyError != nil {
			return saveKeyError
		}
	}

	privValFile := config.GetString("priv_validator_file_root")
	validator.SetFile(privValFile + ".json")
	validator.Save()

	err := initEthGenesisFromExistValidator(chainId, config, validators)
	if err != nil {
		return err
	}

	init_neatchain(chainId, config.GetString("neat_genesis_file"), ctx)

	init_em_files(config, chainId, config.GetString("neat_genesis_file"), validators)

	return nil
}
