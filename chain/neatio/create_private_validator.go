package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neatio-net/crypto-go"
	"github.com/neatio-net/neatio/chain/consensus/neatcon/types"
	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/params"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/utils"
	"github.com/neatio-net/wire-go"
	"gopkg.in/urfave/cli.v1"
)

type PrivValidatorForConsole struct {
	Address string `json:"address"`

	PubKey crypto.PubKey `json:"consensus_pub_key"`

	PrivKey crypto.PrivKey `json:"consensus_priv_key"`
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

	validator := types.GenPrivValidatorKey(common.HexToAddress(address))

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
