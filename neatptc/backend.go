// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package neatptc implements the Ethereum protocol.
package neatptc

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/neatlab/neatio/accounts"
	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/common/hexutil"
	"github.com/neatlab/neatio/consensus"
	"github.com/neatlab/neatio/consensus/neatpos"
	neatconBackend "github.com/neatlab/neatio/consensus/neatpos"
	"github.com/neatlab/neatio/core"
	"github.com/neatlab/neatio/core/bloombits"
	"github.com/neatlab/neatio/core/datareduction"
	"github.com/neatlab/neatio/core/rawdb"
	"github.com/neatlab/neatio/core/types"
	"github.com/neatlab/neatio/core/vm"
	"github.com/neatlab/neatio/event"
	"github.com/neatlab/neatio/internal/neatapi"
	"github.com/neatlab/neatio/log"
	"github.com/neatlab/neatio/miner"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/neatptc/downloader"
	"github.com/neatlab/neatio/neatptc/filters"
	"github.com/neatlab/neatio/neatptc/gasprice"
	"github.com/neatlab/neatio/node"
	"github.com/neatlab/neatio/p2p"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/rlp"
	"github.com/neatlab/neatio/rpc"
	"gopkg.in/urfave/cli.v1"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *core.ChainIndexer)
}

// NeatChain implements the Neatio full node service.
type NeatChain struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan chan bool // Channel for shutting down the neatChain

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager

	// DB interfaces
	chainDb neatdb.Database // Block chain database
	pruneDb neatdb.Database // Prune data database

	eventMux       *event.TypeMux
	engine         consensus.NeatPoS
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	ApiBackend *EthApiBackend

	miner    *miner.Miner
	gasPrice *big.Int
	coinbase common.Address
	solcPath string

	networkId     uint64
	netRPCService *neatapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and etherbase)
}

// New creates a new Neatio object (including the
// initialisation of the common Neatio object)
func New(ctx *node.ServiceContext, config *Config, cliCtx *cli.Context,
	cch core.CrossChainHelper, logger log.Logger, isTestnet bool) (*NeatChain, error) {

	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := ctx.OpenDatabase("chaindata", config.DatabaseCache, config.DatabaseHandles, "neatio/db/chaindata/")
	if err != nil {
		return nil, err
	}
	pruneDb, err := ctx.OpenDatabase("prunedata", config.DatabaseCache, config.DatabaseHandles, "neatio/db/prune/")
	if err != nil {
		return nil, err
	}

	isMainChain := params.IsMainChain(ctx.ChainId())

	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlockWithDefault(chainDb, config.Genesis, isMainChain, isTestnet)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	chainConfig.ChainLogger = logger
	logger.Info("Initialised chain configuration", "network", ctx.ChainId())

	neatChain := &NeatChain{
		config:         config,
		chainDb:        chainDb,
		pruneDb:        pruneDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, config, chainConfig, chainDb, cliCtx, cch),
		shutdownChan:   make(chan bool),
		networkId:      config.NetworkId,
		gasPrice:       config.MinerGasPrice,
		coinbase:       config.Coinbase,
		solcPath:       config.SolcPath,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	//logger.Info("Initialising NeatChain protocol", "versions", eth.engine.Protocol().Versions, "network", config.NetworkId, "dbversion", dbVer)
	logger.Info("Initialising neatio protocol", "network-port", config.NetworkId, "db-version", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			logger.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	var (
		vmConfig    = vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit: config.TrieCleanCache,

			TrieDirtyLimit:    config.TrieDirtyCache,
			TrieDirtyDisabled: config.NoPruning,
			TrieTimeLimit:     config.TrieTimeout,
		}
	)
	//eth.engine = CreateConsensusEngine(ctx, config, chainConfig, chainDb, cliCtx, cch)

	neatChain.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, neatChain.chainConfig, neatChain.engine, vmConfig, cch)
	if err != nil {
		return nil, err
	}

	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		logger.Warn("Rewinding chain to upgrade configuration", "err", compat)
		neatChain.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	neatChain.bloomIndexer.Start(neatChain.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	neatChain.txPool = core.NewTxPool(config.TxPool, neatChain.chainConfig, neatChain.blockchain, cch)

	if neatChain.protocolManager, err = NewProtocolManager(neatChain.chainConfig, config.SyncMode, config.NetworkId, neatChain.eventMux, neatChain.txPool, neatChain.engine, neatChain.blockchain, chainDb, cch); err != nil {
		return nil, err
	}
	neatChain.miner = miner.New(neatChain, neatChain.chainConfig, neatChain.EventMux(), neatChain.engine, config.MinerGasFloor, config.MinerGasCeil, cch)
	neatChain.miner.SetExtra(makeExtraData(config.ExtraData))

	neatChain.ApiBackend = &EthApiBackend{neatChain, nil, cch}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.MinerGasPrice
	}
	neatChain.ApiBackend.gpo = gasprice.NewOracle(neatChain.ApiBackend, gpoParams)

	return neatChain, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"neatio",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateConsensusEngine creates the required type of consensus engine instance for an NeatChain service
func CreateConsensusEngine(ctx *node.ServiceContext, config *Config, chainConfig *params.ChainConfig, db neatdb.Database,
	cliCtx *cli.Context, cch core.CrossChainHelper) consensus.NeatPoS {
	// If NeatCon is requested, set it up
	if chainConfig.NeatPoS.Epoch != 0 {
		config.NeatPoS.Epoch = chainConfig.NeatPoS.Epoch
	}
	config.NeatPoS.ProposerPolicy = neatpos.ProposerPolicy(chainConfig.NeatPoS.ProposerPolicy)
	return neatconBackend.New(chainConfig, cliCtx, ctx.NodeKey(), cch)
}

// APIs returns the collection of RPC services the NeatChain package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *NeatChain) APIs() []rpc.API {

	apis := neatapi.GetAPIs(s.ApiBackend, s.solcPath)
	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)
	// Append all the local APIs and return
	apis = append(apis, []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(s),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
	return apis
}

func (s *NeatChain) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *NeatChain) Coinbase() (eb common.Address, err error) {
	if neatpos, ok := s.engine.(consensus.NeatPoS); ok {
		eb = neatpos.PrivateValidator()
		if eb != (common.Address{}) {
			return eb, nil
		} else {
			return eb, errors.New("private validator missing")
		}
	} else {
		s.lock.RLock()
		coinbase := s.coinbase
		s.lock.RUnlock()

		if coinbase != (common.Address{}) {
			return coinbase, nil
		}
		if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
			if accounts := wallets[0].Accounts(); len(accounts) > 0 {
				coinbase := accounts[0].Address

				s.lock.Lock()
				s.coinbase = coinbase
				s.lock.Unlock()

				log.Info("Coinbase automatically configured", "address", coinbase)
				return coinbase, nil
			}
		}
	}
	return common.Address{}, fmt.Errorf("etherbase must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (self *NeatChain) SetCoinbase(coinbase common.Address) {

	self.lock.Lock()
	self.coinbase = coinbase
	self.lock.Unlock()

	self.miner.SetCoinbase(coinbase)
}

func (s *NeatChain) StartMining(local bool) error {
	var eb common.Address
	if neatpos, ok := s.engine.(consensus.NeatPoS); ok {
		eb = neatpos.PrivateValidator()
		if (eb == common.Address{}) {
			log.Error("Cannot start mining without private validator")
			return errors.New("private validator missing")
		}
	} else {
		_, err := s.Coinbase()
		if err != nil {
			log.Error("Cannot start mining without etherbase", "err", err)
			return fmt.Errorf("etherbase missing: %v", err)
		}
	}

	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so noone will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *NeatChain) StopMining()         { s.miner.Stop() }
func (s *NeatChain) IsMining() bool      { return s.miner.Mining() }
func (s *NeatChain) Miner() *miner.Miner { return s.miner }

func (s *NeatChain) ChainConfig() *params.ChainConfig   { return s.chainConfig }
func (s *NeatChain) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *NeatChain) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *NeatChain) TxPool() *core.TxPool               { return s.txPool }
func (s *NeatChain) EventMux() *event.TypeMux           { return s.eventMux }
func (s *NeatChain) Engine() consensus.NeatPoS          { return s.engine }
func (s *NeatChain) ChainDb() neatdb.Database           { return s.chainDb }
func (s *NeatChain) IsListening() bool                  { return true } // Always listening
func (s *NeatChain) EthVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *NeatChain) NetVersion() uint64                 { return s.networkId }
func (s *NeatChain) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *NeatChain) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// NeatChain protocol implementation.
func (s *NeatChain) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers()

	// Start the RPC service
	s.netRPCService = neatapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers

	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)

	// Start the Auto Mining Loop
	go s.loopForMiningEvent()

	// Start the Data Reduction
	if s.config.PruneStateData && s.chainConfig.NeatChainId == "side_0" {
		go s.StartScanAndPrune(0)
	}

	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// NeatChain protocol.
func (s *NeatChain) Stop() error {
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.protocolManager.Stop()
	s.txPool.Stop()
	s.miner.Stop()
	s.engine.Close()
	s.miner.Close()
	s.eventMux.Stop()

	s.chainDb.Close()
	s.pruneDb.Close()
	close(s.shutdownChan)

	return nil
}

func (s *NeatChain) loopForMiningEvent() {
	// Start/Stop mining Feed
	startMiningCh := make(chan core.StartMiningEvent, 1)
	startMiningSub := s.blockchain.SubscribeStartMiningEvent(startMiningCh)

	stopMiningCh := make(chan core.StopMiningEvent, 1)
	stopMiningSub := s.blockchain.SubscribeStopMiningEvent(stopMiningCh)

	defer startMiningSub.Unsubscribe()
	defer stopMiningSub.Unsubscribe()

	for {
		select {
		case <-startMiningCh:
			if !s.IsMining() {
				s.lock.RLock()
				price := s.gasPrice
				s.lock.RUnlock()
				s.txPool.SetGasPrice(price)
				s.chainConfig.ChainLogger.Info("NeatPoS Consensus Engine will be start shortly")
				s.engine.(consensus.NeatPoS).ForceStart()
				s.StartMining(true)
			} else {
				s.chainConfig.ChainLogger.Info("NeatPoS Consensus Engine already started")
			}
		case <-stopMiningCh:
			if s.IsMining() {
				s.chainConfig.ChainLogger.Info("NeatPoS Consensus Engine will be stop shortly")
				s.StopMining()
			} else {
				s.chainConfig.ChainLogger.Info("NeatPoS Consensus Engine already stopped")
			}
		case <-startMiningSub.Err():
			return
		case <-stopMiningSub.Err():
			return
		}
	}
}

func (s *NeatChain) StartScanAndPrune(blockNumber uint64) {

	if datareduction.StartPruning() {
		log.Info("Data Reduction - Start")
	} else {
		log.Info("Data Reduction - Pruning is already running")
		return
	}

	latestBlockNumber := s.blockchain.CurrentHeader().Number.Uint64()
	if blockNumber == 0 || blockNumber >= latestBlockNumber {
		blockNumber = latestBlockNumber
		log.Infof("Data Reduction - Last block number %v", blockNumber)
	} else {
		log.Infof("Data Reduction - User defined Last block number %v", blockNumber)
	}

	ps := rawdb.ReadHeadScanNumber(s.pruneDb)
	var scanNumber uint64
	if ps != nil {
		scanNumber = *ps
	}

	pp := rawdb.ReadHeadPruneNumber(s.pruneDb)
	var pruneNumber uint64
	if pp != nil {
		pruneNumber = *pp
	}
	log.Infof("Data Reduction - Last scan number %v, prune number %v", scanNumber, pruneNumber)

	pruneProcessor := datareduction.NewPruneProcessor(s.chainDb, s.pruneDb, s.blockchain, s.config.PruneBlockData)

	lastScanNumber, lastPruneNumber := pruneProcessor.Process(blockNumber, scanNumber, pruneNumber)
	log.Infof("Data Reduction - After prune, last number scan %v, prune number %v", lastScanNumber, lastPruneNumber)
	if s.config.PruneBlockData {
		for i := uint64(1); i < lastPruneNumber; i++ {
			rawdb.DeleteBody(s.chainDb, rawdb.ReadCanonicalHash(s.chainDb, i), i)
		}
		log.Infof("deleted block from 1 to %v", lastPruneNumber)
	}
	log.Info("Data Reduction - Completed")

	datareduction.StopPruning()
}
