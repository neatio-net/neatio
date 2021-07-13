package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neatlab/neatio/cmd/utils"
	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/consensus/neatpos/types"
	"github.com/neatlab/neatio/log"
	"github.com/neatlab/neatio/params"
	crypto "github.com/neatlib/crypto-go"
	wire "github.com/neatlib/wire-go"
	"gopkg.in/urfave/cli.v1"
)

type PrivValidatorForConsole struct {
	// NeatChain Account Address
	Address string `json:"address"`
	// NeatChain Consensus Public Key, in BLS format
	PubKey crypto.PubKey `json:"validator_pub_key"`
	// NeatChain Consensus Private Key, in BLS format
	// PrivKey should be empty if a Signer other than the default is being used.
	PrivKey crypto.PrivKey `json:"validator_priv_key"`
}

func CreatePrivateValidatorCmd(ctx *cli.Context) error {
	var consolePrivVal *PrivValidatorForConsole
	address := ctx.Args().First()

	if address == "" {
		log.Info("address is empty, need an address")
		return nil
	}

	datadir := ctx.GlobalString(utils.DataDirFlag.Name)
	if err := os.MkdirAll(datadir, 0700); err != nil {
		return err
	}

	chainId := params.MainnetChainConfig.NeatChainId

	if ctx.GlobalIsSet(utils.TestnetFlag.Name) {
		chainId = params.TestnetChainConfig.NeatChainId
	}

	privValFilePath := filepath.Join(ctx.GlobalString(utils.DataDirFlag.Name), chainId)
	privValFile := filepath.Join(ctx.GlobalString(utils.DataDirFlag.Name), chainId, "priv_validator.json")

	err := os.MkdirAll(privValFilePath, os.ModePerm)
	if err != nil {
		panic(err)
	}

	validator := types.GenPrivValidatorKey(common.StringToAddress(address))

	consolePrivVal = &PrivValidatorForConsole{
		Address: validator.Address.String(),
		PubKey:  validator.PubKey,
		PrivKey: validator.PrivKey,
	}

	fmt.Printf(string(wire.JSONBytesPretty(consolePrivVal)))
	validator.SetFile(privValFile)
	validator.Save()

	return nil
}
