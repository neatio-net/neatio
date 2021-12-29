package main

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neatlab/neatio/params"
)

const (
	ipcAPIs  = "admin:1.0 debug:1.0 eth:1.0 miner:1.0 net:1.0 personal:1.0 rpc:1.0 shh:1.0 txpool:1.0 web3:1.0"
	httpAPIs = "eth:1.0 net:1.0 rpc:1.0 web3:1.0"
)

func TestConsoleWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"

	neatio := runneatchain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--shh",
		"console")

	neatio.SetTemplateFunc("goos", func() string { return runtime.GOOS })
	neatio.SetTemplateFunc("goarch", func() string { return runtime.GOARCH })
	neatio.SetTemplateFunc("gover", runtime.Version)
	neatio.SetTemplateFunc("neatiover", func() string { return params.Version })
	neatio.SetTemplateFunc("niltime", func() string { return time.Unix(0, 0).Format(time.RFC1123) })
	neatio.SetTemplateFunc("apis", func() string { return ipcAPIs })

	neatio.Expect(`
Welcome to the neatio JavaScript console!

instance: neatio/v{{neatiover}}/{{goos}}-{{goarch}}/{{gover}}
coinbase: {{.Etherbase}}
at block: 0 ({{niltime}})
 datadir: {{.Datadir}}
 modules: {{apis}}

> {{.InputLine "exit"}}
`)
	neatio.ExpectExit()
}

func TestIPCAttachWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	var ipc string
	if runtime.GOOS == "windows" {
		ipc = `\\.\pipe\neatio` + strconv.Itoa(trulyRandInt(100000, 999999))
	} else {
		ws := tmpdir(t)
		defer os.RemoveAll(ws)
		ipc = filepath.Join(ws, "neatio.ipc")
	}
	neatio := runneatchain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--shh", "--ipcpath", ipc)

	time.Sleep(2 * time.Second)
	testAttachWelcome(t, neatio, "ipc:"+ipc, ipcAPIs)

	neatio.Interrupt()
	neatio.ExpectExit()
}

func TestHTTPAttachWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	port := strconv.Itoa(trulyRandInt(1024, 65536))
	neatio := runneatchain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--rpc", "--rpcport", port)

	time.Sleep(2 * time.Second)
	testAttachWelcome(t, neatio, "http://localhost:"+port, httpAPIs)

	neatio.Interrupt()
	neatio.ExpectExit()
}

func TestWSAttachWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	port := strconv.Itoa(trulyRandInt(1024, 65536))

	neatio := runneatchain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--ws", "--wsport", port)

	time.Sleep(2 * time.Second)
	testAttachWelcome(t, neatio, "ws://localhost:"+port, httpAPIs)

	neatio.Interrupt()
	neatio.ExpectExit()
}

func testAttachWelcome(t *testing.T, neatio *testneatchain, endpoint, apis string) {
	attach := runneatchain(t, "attach", endpoint)
	defer attach.ExpectExit()
	attach.CloseStdin()

	attach.SetTemplateFunc("goos", func() string { return runtime.GOOS })
	attach.SetTemplateFunc("goarch", func() string { return runtime.GOARCH })
	attach.SetTemplateFunc("gover", runtime.Version)
	attach.SetTemplateFunc("neatiover", func() string { return params.Version })
	attach.SetTemplateFunc("etherbase", func() string { return neatio.Etherbase })
	attach.SetTemplateFunc("niltime", func() string { return time.Unix(0, 0).Format(time.RFC1123) })
	attach.SetTemplateFunc("ipc", func() bool { return strings.HasPrefix(endpoint, "ipc") })
	attach.SetTemplateFunc("datadir", func() string { return neatio.Datadir })
	attach.SetTemplateFunc("apis", func() string { return apis })

	attach.Expect(`
Welcome to the neatio JavaScript console!

instance: neatio/v{{neatiover}}/{{goos}}-{{goarch}}/{{gover}}
coinbase: {{etherbase}}
at block: 0 ({{niltime}}){{if ipc}}
 datadir: {{datadir}}{{end}}
 modules: {{apis}}

> {{.InputLine "exit" }}
`)
	attach.ExpectExit()
}

func trulyRandInt(lo, hi int) int {
	num, _ := rand.Int(rand.Reader, big.NewInt(int64(hi-lo)))
	return int(num.Int64()) + lo
}
