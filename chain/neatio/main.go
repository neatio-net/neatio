// neatio is the official command-line client for NEAT Blockchain.
package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/neatlab/neatio/chain/log"

	"github.com/neatlab/neatio/internal/debug"
	"github.com/neatlab/neatio/utilities/console"
	"github.com/neatlab/neatio/utilities/metrics"
	"github.com/neatlab/neatio/utilities/utils"
	"gopkg.in/urfave/cli.v1"
)

const (
	clientIdentifier = "neatio" // Client identifier to advertise over the network
)

var (
	// Git SHA1 commit hash of the release (set via linker flags)
	gitCommit = ""
	gitDate   = ""

	// The app that holds all commands and flags.
	app = utils.NewApp(gitCommit, "the neatio command line interface")
	// flags that configure the node
	nodeFlags = []cli.Flag{
		utils.IdentityFlag,
		utils.UnlockedAccountFlag,
		utils.PasswordFileFlag,
		utils.BootnodesFlag,
		utils.BootnodesV4Flag,
		utils.BootnodesV5Flag,
		utils.DataDirFlag,
		utils.KeyStoreDirFlag,
		utils.NoUSBFlag,
		utils.TxPoolNoLocalsFlag,
		utils.TxPoolJournalFlag,
		utils.TxPoolRejournalFlag,
		utils.TxPoolPriceLimitFlag,
		utils.TxPoolPriceBumpFlag,
		utils.TxPoolAccountSlotsFlag,
		utils.TxPoolGlobalSlotsFlag,
		utils.TxPoolAccountQueueFlag,
		utils.TxPoolGlobalQueueFlag,
		utils.TxPoolLifetimeFlag,
		//utils.FastSyncFlag,
		utils.SyncModeFlag,
		utils.GCModeFlag,
		utils.CacheFlag,
		utils.CacheDatabaseFlag,
		utils.CacheTrieFlag,
		utils.CacheGCFlag,
		utils.ListenPortFlag,
		utils.MaxPeersFlag,
		utils.MaxPendingPeersFlag,
		utils.MiningEnabledFlag,
		utils.MinerThreadsFlag,
		utils.MinerGasTargetFlag,
		utils.MinerGasLimitFlag,
		utils.MinerGasPriceFlag,
		utils.MinerCoinbaseFlag,
		utils.NATFlag,
		utils.NoDiscoverFlag,
		utils.DiscoveryV5Flag,
		utils.NetrestrictFlag,
		utils.NodeKeyFileFlag,
		utils.NodeKeyHexFlag,
		utils.TestnetFlag,
		utils.VMEnableDebugFlag,
		utils.NetworkIdFlag,
		utils.RPCCORSDomainFlag,
		utils.RPCVirtualHostsFlag,
		//utils.EthStatsURLFlag,
		utils.MetricsEnabledFlag,
		utils.NoCompactionFlag,
		utils.GpoBlocksFlag,
		utils.GpoPercentileFlag,
		utils.ExtraDataFlag,
		//configFileFlag,

		//utils.LogDirFlag,
		utils.SideChainFlag,
	}

	rpcFlags = []cli.Flag{
		utils.RPCEnabledFlag,
		utils.RPCListenAddrFlag,
		utils.RPCPortFlag,
		utils.RPCApiFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.WSPortFlag,
		utils.WSApiFlag,
		utils.WSAllowedOriginsFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
	}
)

func init() {
	// Initialize the CLI app and start neatio
	app.Action = neatchainCmd
	app.HideVersion = true // we have a command to print the version
	app.Copyright = "Copyright 2021 Neatio Developers"
	app.Commands = []cli.Command{
		// See chaincmd.go:
		createValidatorCmd,
		initNEATGenesisCmd,
		initCommand,
		//initSideChainCmd,
		importCommand,
		exportCommand,
		copydbCommand,
		removedbCommand,
		dumpCommand,
		// See monitorcmd.go:
		monitorCommand,
		// See accountcmd.go:
		accountCommand,
		//walletCommand,
		// See consolecmd.go:
		consoleCommand,
		attachCommand,
		javascriptCommand,
		// See misccmd.go:

		bugCommand,
		// See config.go
		dumpConfigCommand,
		versionCommand,
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, consoleFlags...)
	app.Flags = append(app.Flags, debug.Flags...)

	app.Before = func(ctx *cli.Context) error {
		runtime.GOMAXPROCS(runtime.NumCPU())

		// Setup the Global Logger

		log.NewLogger("", "", ctx.GlobalInt("verbosity"), ctx.GlobalBool("debug"), ctx.GlobalString("vmodule"), ctx.GlobalString("backtrace"))

		if err := debug.Setup(ctx); err != nil {
			return err
		}

		// Start system runtime metrics collection
		go metrics.CollectProcessMetrics(3 * time.Second)

		return nil
	}

	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		console.Stdin.Close() // Resets terminal mode.
		return nil
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
