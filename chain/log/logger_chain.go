package log

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/mattn/go-colorable"
)

var loggerMap sync.Map

func newChainLogger(chainID string) Logger {

	chainLogger := &logger{[]interface{}{}, new(swapHandler)}

	loggerMap.Store(chainID, chainLogger)

	return chainLogger
}

func NewLogger(chainID, logDir string, logLevel int, fileLine bool, vmodule, backtrace string) Logger {

	PrintOrigins(fileLine)

	output := colorable.NewColorableStdout()
	ostream := StreamHandler(output, TerminalFormat(true))
	glogger := NewGlogHandler(ostream)

	if logDir != "" {

		preimages_logDir := filepath.Join(logDir, "preimages")
		if err := os.MkdirAll(preimages_logDir, 0700); err != nil {
			panic(err)
		}
		preimageFileHandler := Must.FileHandler(filepath.Join(preimages_logDir, "preimages.log"), TerminalFormat(false))
		preimageFilter := MatchFilterHandler("module", "preimages", preimageFileHandler)

		rfh, err := RotatingFileHandler(
			logDir,
			10*1024*1024,
			TerminalFormat(false),
		)
		if err != nil {
			panic(err)
		}
		glogger.SetHandler(MultiHandler(ostream, rfh, preimageFilter))
	}
	glogger.Verbosity(Lvl(logLevel))
	glogger.Vmodule(vmodule)
	glogger.BacktraceAt(backtrace)

	var logger Logger
	if chainID == "" {
		logger = Root()
		logger.SetHandler(glogger)
	} else {
		logger = newChainLogger(chainID)
		logger.SetHandler(glogger)
	}

	return logger
}

func GetLogger(chainID string) Logger {
	logger, find := loggerMap.Load(chainID)
	if find {
		return logger.(Logger)
	} else {
		return nil
	}
}

func RangeLogger(f func(key, value interface{}) bool) {
	loggerMap.Range(f)
}
