package neatptc

import (
	"math/big"
	"os"
	"os/user"

	"runtime"
	"time"

	"github.com/neatlab/neatio/chain/consensus/neatcon"
	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/neatptc/downloader"
	"github.com/neatlab/neatio/neatptc/gasprice"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/hexutil"
)

var DefaultConfig = Config{

	SyncMode: downloader.FullSync,

	NetworkId:      9910,
	DatabaseCache:  512,
	TrieCleanCache: 256,
	TrieDirtyCache: 256,
	TrieTimeout:    60 * time.Minute,
	MinerGasFloor:  120000000,
	MinerGasCeil:   120000000,
	MinerGasPrice:  big.NewInt(params.GWei),

	TxPool: core.DefaultTxPoolConfig,
	GPO: gasprice.Config{
		Blocks:     20,
		Percentile: 60,
	},
}

func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "windows" {

	} else {

	}
}

type Config struct {
	Genesis *core.Genesis `toml:",omitempty"`

	NetworkId uint64
	SyncMode  downloader.SyncMode

	NoPruning bool

	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int

	TrieCleanCache int
	TrieDirtyCache int
	TrieTimeout    time.Duration

	Coinbase      common.Address `toml:",omitempty"`
	ExtraData     []byte         `toml:",omitempty"`
	MinerGasFloor uint64
	MinerGasCeil  uint64
	MinerGasPrice *big.Int

	SolcPath string

	TxPool core.TxPoolConfig

	GPO gasprice.Config

	EnablePreimageRecording bool

	NeatCon neatcon.Config

	DocRoot string `toml:"-"`

	PruneStateData bool
	PruneBlockData bool
}

type configMarshaling struct {
	ExtraData hexutil.Bytes
}
