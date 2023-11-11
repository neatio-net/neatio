package miner

import (
	"fmt"
	"sync/atomic"

	"github.com/neatio-net/neatio/chain/accounts"
	"github.com/neatio-net/neatio/chain/consensus"
	"github.com/neatio-net/neatio/chain/core"
	"github.com/neatio-net/neatio/chain/core/state"
	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/neatdb"
	"github.com/neatio-net/neatio/neatptc/downloader"
	"github.com/neatio-net/neatio/params"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/event"
)

type Backend interface {
	AccountManager() *accounts.Manager
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
	ChainDb() neatdb.Database
}

type Pending interface {
	Pending() (*types.Block, *state.StateDB)
	PendingBlock() *types.Block
}

type Miner struct {
	mux *event.TypeMux

	worker *worker

	coinbase common.Address
	mining   int32
	eth      Backend
	engine   consensus.Engine
	exitCh   chan struct{}

	canStart    int32
	shouldStart int32

	logger log.Logger
	cch    core.CrossChainHelper
}

func New(eth Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, gasFloor, gasCeil uint64, cch core.CrossChainHelper) *Miner {
	miner := &Miner{
		eth:      eth,
		mux:      mux,
		engine:   engine,
		exitCh:   make(chan struct{}),
		worker:   newWorker(config, engine, eth, mux, gasFloor, gasCeil, cch),
		canStart: 1,
		logger:   config.ChainLogger,
		cch:      cch,
	}
	miner.Register(NewCpuAgent(eth.BlockChain(), engine, config.ChainLogger))
	go miner.update()

	return miner
}

func (self *Miner) update() {
	events := self.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer events.Unsubscribe()

	for {
		select {
		case ev := <-events.Chan():
			if ev == nil {
				return
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				self.logger.Debug("(self *Miner) update(); downloader.StartEvent received")
				atomic.StoreInt32(&self.canStart, 0)
				if self.Mining() {
					self.Stop()
					atomic.StoreInt32(&self.shouldStart, 1)
					self.logger.Info("Mining aborted due to sync")
				}
			case downloader.DoneEvent, downloader.FailedEvent:

				self.logger.Debug("(self *Miner) update(); downloader.DoneEvent, downloader.FailedEvent received")
				shouldStart := atomic.LoadInt32(&self.shouldStart) == 1

				atomic.StoreInt32(&self.canStart, 1)
				atomic.StoreInt32(&self.shouldStart, 0)
				if shouldStart {
					self.Start(self.coinbase)
				}

				return
			}
		case <-self.exitCh:
			return
		}
	}
}

func (self *Miner) Start(coinbase common.Address) {
	atomic.StoreInt32(&self.shouldStart, 1)
	self.SetCoinbase(coinbase)

	if atomic.LoadInt32(&self.canStart) == 0 {
		self.logger.Info("Network syncing, will start miner afterwards")
		return
	}
	self.worker.start()
	self.worker.commitNewWork()
}

func (self *Miner) Stop() {
	self.worker.stop()
	atomic.StoreInt32(&self.shouldStart, 0)
}

func (self *Miner) Close() {
	self.worker.close()
	close(self.exitCh)
}

func (self *Miner) Register(agent Agent) {
	if self.Mining() {
		agent.Start()
	}
	self.worker.register(agent)
}

func (self *Miner) Unregister(agent Agent) {
	self.worker.unregister(agent)
}

func (self *Miner) Mining() bool {
	return self.worker.isRunning()
}

func (self *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	self.worker.setExtra(extra)
	return nil
}

func (self *Miner) Pending() (*types.Block, *state.StateDB) {
	return self.worker.pending()
}

func (self *Miner) PendingBlock() *types.Block {
	return self.worker.pendingBlock()
}

func (self *Miner) SetCoinbase(addr common.Address) {
	self.coinbase = addr
	self.worker.setCoinbase(addr)
}
