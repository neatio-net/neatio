package neatcon

import (
	"crypto/ecdsa"
	"sync"

	"github.com/neatio-network/neatio/chain/consensus"
	"github.com/neatio-network/neatio/chain/consensus/neatcon/types"
	"github.com/neatio-network/neatio/chain/core"
	neatTypes "github.com/neatio-network/neatio/chain/core/types"
	"github.com/neatio-network/neatio/chain/log"
	"github.com/neatio-network/neatio/params"
	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/event"
	"gopkg.in/urfave/cli.v1"
)

func New(chainConfig *params.ChainConfig, cliCtx *cli.Context,
	privateKey *ecdsa.PrivateKey, cch core.CrossChainHelper) consensus.NeatCon {

	config := GetNeatConConfig(chainConfig.NeatChainId, cliCtx)

	backend := &backend{
		chainConfig:     chainConfig,
		neatconEventMux: new(event.TypeMux),
		privateKey:      privateKey,
		logger:          chainConfig.ChainLogger,
		commitCh:        make(chan *neatTypes.Block, 1),
		vcommitCh:       make(chan *types.IntermediateBlockResult, 1),
		coreStarted:     false,
	}
	backend.core = MakeNeatConNode(backend, config, chainConfig, cch)
	return backend
}

type backend struct {
	chainConfig     *params.ChainConfig
	neatconEventMux *event.TypeMux
	privateKey      *ecdsa.PrivateKey
	address         common.Address
	core            *Node
	logger          log.Logger
	chain           consensus.ChainReader
	currentBlock    func() *neatTypes.Block
	hasBadBlock     func(hash common.Hash) bool

	commitCh          chan *neatTypes.Block
	vcommitCh         chan *types.IntermediateBlockResult
	proposedBlockHash common.Hash
	sealMu            sync.Mutex
	shouldStart       bool
	coreStarted       bool
	coreMu            sync.RWMutex

	broadcaster consensus.Broadcaster
}

func GetBackend() backend {
	return backend{}
}
