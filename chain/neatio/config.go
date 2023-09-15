package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"unicode"

	"github.com/nio-net/nio/chain/core"
	"github.com/nio-net/nio/chain/log"

	"gopkg.in/urfave/cli.v1"

	"github.com/naoina/toml"
	neatptc "github.com/nio-net/nio/neatptc"
	"github.com/nio-net/nio/network/node"
	"github.com/nio-net/nio/params"
	"github.com/nio-net/nio/utilities/utils"
)

var (
	dumpConfigCommand = cli.Command{
		Action:      utils.MigrateFlags(dumpConfig),
		Name:        "dumpconfig",
		Usage:       "Show configuration values",
		ArgsUsage:   "",
		Category:    "MISCELLANEOUS COMMANDS",
		Description: `The dumpconfig command shows configuration values.`,
	}

	configFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
)

var tomlSettings = toml.Config{
	NormFieldName: func(rt reflect.Type, key string) string {
		return key
	},
	FieldToKey: func(rt reflect.Type, field string) string {
		return field
	},
	MissingField: func(rt reflect.Type, field string) error {
		link := ""
		if unicode.IsUpper(rune(rt.Name()[0])) && rt.PkgPath() != "main" {
			link = fmt.Sprintf(", see https:godoc.org/%s#%s for available fields", rt.PkgPath(), rt.Name())
		}
		return fmt.Errorf("field '%s' is not defined in %s%s", field, rt.String(), link)
	},
}

type ethstatsConfig struct {
	URL string `toml:",omitempty"`
}

type gethConfig struct {
	Eth      neatptc.Config
	Node     node.Config
	Ethstats ethstatsConfig
}

func loadConfig(file string, cfg *gethConfig) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tomlSettings.NewDecoder(bufio.NewReader(f)).Decode(cfg)

	if _, ok := err.(*toml.LineError); ok {
		err = errors.New(file + ", " + err.Error())
	}
	return err
}

func defaultNodeConfig() node.Config {
	cfg := node.DefaultConfig
	cfg.Name = clientIdentifier
	cfg.Version = params.VersionWithCommit(gitCommit)
	cfg.HTTPModules = append(cfg.HTTPModules, "neat", "eth")
	cfg.WSModules = append(cfg.WSModules, "neat", "eth")
	cfg.IPCPath = "neatio.ipc"
	return cfg
}

func makeConfigNode(ctx *cli.Context, chainId string) (*node.Node, gethConfig) {

	cfg := gethConfig{
		Eth:  neatptc.DefaultConfig,
		Node: defaultNodeConfig(),
	}

	if file := ctx.GlobalString(configFileFlag.Name); file != "" {
		if err := loadConfig(file, &cfg); err != nil {
			utils.Fatalf("%v", err)
		}
	}

	cfg.Node.ChainId = chainId

	cfg.Node.Logger = log.NewLogger(chainId, "", ctx.GlobalInt("verbosity"), ctx.GlobalBool("debug"), ctx.GlobalString("vmodule"), ctx.GlobalString("backtrace"))

	utils.SetNodeConfig(ctx, &cfg.Node)
	stack, err := node.New(&cfg.Node)
	if err != nil {
		utils.Fatalf("Failed to create the protocol stack: %v", err)
	}
	utils.SetEthConfig(ctx, stack, &cfg.Eth)
	if ctx.GlobalIsSet(utils.EthStatsURLFlag.Name) {
		cfg.Ethstats.URL = ctx.GlobalString(utils.EthStatsURLFlag.Name)
	}

	utils.SetGeneralConfig(ctx)

	return stack, cfg
}

func makeFullNode(ctx *cli.Context, cch core.CrossChainHelper, chainId string) *node.Node {
	stack, cfg := makeConfigNode(ctx, chainId)

	utils.RegisterIntService(stack, &cfg.Eth, ctx, cch)

	if err := stack.GatherServices(); err != nil {
		return nil
	} else {
		return stack
	}
}

func dumpConfig(ctx *cli.Context) error {
	_, cfg := makeConfigNode(ctx, clientIdentifier)
	comment := ""

	if cfg.Eth.Genesis != nil {
		cfg.Eth.Genesis = nil
		comment += "# Note: this config doesn't contain the genesis block.\n\n"
	}

	out, err := tomlSettings.Marshal(&cfg)
	if err != nil {
		return err
	}
	io.WriteString(os.Stdout, comment)
	os.Stdout.Write(out)
	return nil
}
