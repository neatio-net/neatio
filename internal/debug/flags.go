package debug

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	colorable "github.com/mattn/go-colorable"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/chain/log/term"
	"github.com/neatlab/neatio/utilities/metrics"
	"github.com/neatlab/neatio/utilities/metrics/exp"
	"gopkg.in/urfave/cli.v1"
)

var (
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Usage: "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		Value: 3,
	}
	vmoduleFlag = cli.StringFlag{
		Name:  "vmodule",
		Usage: "Per-module verbosity: comma-separated list of <pattern>=<level> (e.g. eth/*=5,p2p=4)",
		Value: "",
	}
	backtraceAtFlag = cli.StringFlag{
		Name:  "backtrace",
		Usage: "Request a stack trace at a specific logging statement (e.g. \"block.go:271\")",
		Value: "",
	}
	debugFlag = cli.BoolFlag{
		Name:  "debug",
		Usage: "Prepends log messages with call-site location (file and line number)",
	}
	pprofFlag = cli.BoolFlag{
		Name:  "pprof",
		Usage: "Enable the pprof HTTP server",
	}
	pprofPortFlag = cli.IntFlag{
		Name:  "pprofport",
		Usage: "pprof HTTP server listening port",
		Value: 6060,
	}
	pprofAddrFlag = cli.StringFlag{
		Name:  "pprofaddr",
		Usage: "pprof HTTP server listening interface",
		Value: "127.0.0.1",
	}
	memprofilerateFlag = cli.IntFlag{
		Name:  "memprofilerate",
		Usage: "Turn on memory profiling with the given rate",
		Value: runtime.MemProfileRate,
	}
	blockprofilerateFlag = cli.IntFlag{
		Name:  "blockprofilerate",
		Usage: "Turn on block profiling with the given rate",
	}
	cpuprofileFlag = cli.StringFlag{
		Name:  "cpuprofile",
		Usage: "Write CPU profile to the given file",
	}
	traceFlag = cli.StringFlag{
		Name:  "trace",
		Usage: "Write execution trace to the given file",
	}
)

var Flags = []cli.Flag{
	verbosityFlag, vmoduleFlag, backtraceAtFlag, debugFlag,
	pprofFlag, pprofAddrFlag, pprofPortFlag,
	memprofilerateFlag, blockprofilerateFlag, cpuprofileFlag, traceFlag,
}

var (
	ostream log.Handler
	glogger *log.GlogHandler
)

func init() {
	usecolor := term.IsTty(os.Stderr.Fd()) && os.Getenv("TERM") != "dumb"
	output := io.Writer(os.Stderr)
	if usecolor {
		output = colorable.NewColorableStderr()
	}
	ostream = log.StreamHandler(output, log.TerminalFormat(usecolor))
	glogger = log.NewGlogHandler(ostream)
}

func Setup(ctx *cli.Context) error {
	runtime.MemProfileRate = ctx.GlobalInt(memprofilerateFlag.Name)
	Handler.SetBlockProfileRate(ctx.GlobalInt(blockprofilerateFlag.Name))
	if traceFile := ctx.GlobalString(traceFlag.Name); traceFile != "" {
		if err := Handler.StartGoTrace(traceFile); err != nil {
			return err
		}
	}
	if cpuFile := ctx.GlobalString(cpuprofileFlag.Name); cpuFile != "" {
		if err := Handler.StartCPUProfile(cpuFile); err != nil {
			return err
		}
	}

	if ctx.GlobalBool(pprofFlag.Name) {
		exp.Exp(metrics.DefaultRegistry)

		address := fmt.Sprintf("%s:%d", ctx.GlobalString(pprofAddrFlag.Name), ctx.GlobalInt(pprofPortFlag.Name))
		go func() {
			log.Info("Starting pprof server", "addr", fmt.Sprintf("http://%s/debug/pprof", address))
			if err := http.ListenAndServe(address, nil); err != nil {
				log.Error("Failure in running pprof server", "err", err)
			}
		}()
	}
	return nil
}

func Exit() {
	Handler.StopCPUProfile()
	Handler.StopGoTrace()
}
