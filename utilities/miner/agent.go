package miner

import (
	"sync"

	"sync/atomic"

	"github.com/neatlab/neatio/chain/consensus"
	ntcTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/log"
)

type CpuAgent struct {
	mu sync.Mutex

	workCh        chan *Work
	stop          chan struct{}
	quitCurrentOp chan struct{}
	returnCh      chan<- *Result

	chain  consensus.ChainReader
	engine consensus.Engine

	isMining int32

	logger log.Logger
}

func NewCpuAgent(chain consensus.ChainReader, engine consensus.Engine, logger log.Logger) *CpuAgent {
	miner := &CpuAgent{
		chain:  chain,
		engine: engine,
		stop:   make(chan struct{}, 1),
		workCh: make(chan *Work, 1),
		logger: logger,
	}
	return miner
}

func (self *CpuAgent) Work() chan<- *Work            { return self.workCh }
func (self *CpuAgent) SetReturnCh(ch chan<- *Result) { self.returnCh = ch }

func (self *CpuAgent) Stop() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 1, 0) {
		return
	}
	self.stop <- struct{}{}
done:

	for {
		select {
		case <-self.workCh:
		default:
			break done
		}
	}
}

func (self *CpuAgent) Start() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 0, 1) {
		return
	}
	go self.update()
}

func (self *CpuAgent) update() {
out:
	for {
		select {
		case work := <-self.workCh:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
			}
			self.quitCurrentOp = make(chan struct{})
			go self.mine(work, self.quitCurrentOp)
			self.mu.Unlock()
		case <-self.stop:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
				self.quitCurrentOp = nil
			}
			self.mu.Unlock()
			break out
		}
	}
}

func (self *CpuAgent) mine(work *Work, stop <-chan struct{}) {
	if result, err := self.engine.Seal(self.chain, work.Block, stop); result != nil {
		switch result := result.(type) {
		case *types.Block:
			self.logger.Info("Successfully sealed new block.", "New height", result.Number())
			self.returnCh <- &Result{Work: work, Block: result}
		case *ntcTypes.IntermediateBlockResult:

			self.returnCh <- &Result{Intermediate: result}
		}

	} else {
		if err != nil {
			self.logger.Warn("Block sealing failed", "err", err)
		}
		// self.logger.Warn("Block sealing aborted")
		self.returnCh <- nil
	}
}
