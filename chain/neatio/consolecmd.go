package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/neatlab/neatio/network/node"
	"github.com/neatlab/neatio/network/rpc"
	"github.com/neatlab/neatio/utilities/console"
	"github.com/neatlab/neatio/utilities/utils"
	"gopkg.in/urfave/cli.v1"
)

var (
	consoleFlags = []cli.Flag{utils.JSpathFlag, utils.ExecFlag, utils.PreloadJSFlag}

	consoleCommand = cli.Command{
		Action:   utils.MigrateFlags(localConsole),
		Name:     "console",
		Usage:    "Start an interactive JavaScript environment",
		Flags:    append(append(nodeFlags, rpcFlags...), consoleFlags...),
		Category: "CONSOLE COMMANDS",
		Description: `
The Geth console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the Ðapp JavaScript API.
See https://github.com/neatlab/neatio/wiki/JavaScript-Console.`,
	}

	attachCommand = cli.Command{
		Action:    utils.MigrateFlags(remoteConsole),
		Name:      "attach",
		Usage:     "Start an interactive JavaScript environment (connect to node)",
		ArgsUsage: "[endpoint]",
		Flags:     append(consoleFlags, utils.DataDirFlag),
		Category:  "CONSOLE COMMANDS",
		Description: `
The Geth console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the Ðapp JavaScript API.
See https://github.com/neatlab/neatio/wiki/JavaScript-Console.
This command allows to open a console on a running neatio node.`,
	}

	javascriptCommand = cli.Command{
		Action:    utils.MigrateFlags(ephemeralConsole),
		Name:      "js",
		Usage:     "Execute the specified JavaScript files",
		ArgsUsage: "<jsfile> [jsfile...]",
		Flags:     append(nodeFlags, consoleFlags...),
		Category:  "CONSOLE COMMANDS",
		Description: `
The JavaScript VM exposes a node admin interface as well as the Ðapp
JavaScript API. See https://github.com/neatlab/neatio/wiki/JavaScript-Console`,
	}
)

func localConsole(ctx *cli.Context) error {
	chainName := ctx.Args().First()
	if chainName == "" {
		utils.Fatalf("This command requires chain name specified.")
	}

	node := makeFullNode(ctx, GetCMInstance(ctx).cch, chainName)
	utils.StartNode(ctx, node)
	defer node.Close()

	client, err := node.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to the inproc neatio: %v", err)
	}
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.GlobalString(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}

	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	if script := ctx.GlobalString(utils.ExecFlag.Name); script != "" {
		console.Evaluate(script)
		return nil
	}
	console.Welcome()
	console.Interactive()

	return nil
}

func remoteConsole(ctx *cli.Context) error {
	endpoint := ctx.Args().First()
	if endpoint == "" {
		path := node.DefaultDataDir()
		if ctx.GlobalIsSet(utils.DataDirFlag.Name) {
			path = ctx.GlobalString(utils.DataDirFlag.Name)
		}
		if path != "" {
			if ctx.GlobalBool(utils.TestnetFlag.Name) {
				path = filepath.Join(path, "testnet")
			}
		}
		endpoint = fmt.Sprintf("%s/neatio.ipc", path)
	}
	client, err := dialRPC(endpoint)
	if err != nil {
		utils.Fatalf("Unable to attach to remote neatio: %v", err)
	}
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.GlobalString(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}

	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	if script := ctx.GlobalString(utils.ExecFlag.Name); script != "" {
		console.Evaluate(script)
		return nil
	}

	console.Welcome()
	console.Interactive()

	return nil
}

func dialRPC(endpoint string) (*rpc.Client, error) {
	if endpoint == "" {
		endpoint = node.DefaultIPCEndpoint(clientIdentifier)
	} else if strings.HasPrefix(endpoint, "rpc:") || strings.HasPrefix(endpoint, "ipc:") {
		endpoint = endpoint[4:]
	}
	return rpc.Dial(endpoint)
}

func ephemeralConsole(ctx *cli.Context) error {
	chainName := ctx.Args().First()
	if chainName == "" {
		utils.Fatalf("This command requires chain name specified.")
	}

	node := makeFullNode(ctx, GetCMInstance(ctx).cch, chainName)
	utils.StartNode(ctx, node)
	defer node.Close()

	client, err := node.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to the inproc neatio: %v", err)
	}
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.GlobalString(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}

	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	for _, file := range ctx.Args() {
		if err = console.Execute(file); err != nil {
			utils.Fatalf("Failed to execute %s: %v", file, err)
		}
	}
	abort := make(chan os.Signal, 1)
	signal.Notify(abort, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-abort
		os.Exit(0)
	}()
	console.Stop(true)

	return nil
}
