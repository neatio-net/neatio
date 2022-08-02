package miner

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neatio-network/neatio/chain/consensus"
	ntcTypes "github.com/neatio-network/neatio/chain/consensus/neatcon/types"
	"github.com/neatio-network/neatio/chain/core"
	"github.com/neatio-network/neatio/chain/core/state"
	"github.com/neatio-network/neatio/chain/core/types"
	"github.com/neatio-network/neatio/chain/core/vm"
	"github.com/neatio-network/neatio/chain/log"
	"github.com/neatio-network/neatio/params"
	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/event"
	"github.com/neatlib/set-go"
)

const (
	resultQueueSize  = 10
	miningLogAtDepth = 5

	txChanSize = 4096

	chainHeadChanSize = 10

	chainSideChanSize = 10
)

type Agent interface {
	Work() chan<- *Work
	SetReturnCh(chan<- *Result)
	Stop()
	Start()
}

type Work struct {
	signer types.Signer

	state     *state.StateDB
	ancestors *set.Set
	family    *set.Set
	uncles    *set.Set
	tcount    int

	Block *types.Block

	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt

	ops *types.PendingOps

	createdAt time.Time
	logger    log.Logger
}

type Result struct {
	Work         *Work
	Block        *types.Block
	Intermediate *ntcTypes.IntermediateBlockResult
}

type worker struct {
	config *params.ChainConfig
	engine consensus.Engine
	eth    Backend
	chain  *core.BlockChain

	gasFloor uint64
	gasCeil  uint64

	mu sync.Mutex

	mux          *event.TypeMux
	txCh         chan core.TxPreEvent
	txSub        event.Subscription
	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription
	chainSideCh  chan core.ChainSideEvent
	chainSideSub event.Subscription
	wg           sync.WaitGroup

	agents   map[Agent]struct{}
	resultCh chan *Result
	exitCh   chan struct{}

	proc core.Validator

	coinbase common.Address
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	uncleMu        sync.Mutex
	possibleUncles map[common.Hash]*types.Block

	unconfirmed *unconfirmedBlocks

	mining int32
	atWork int32

	logger log.Logger
	cch    core.CrossChainHelper
}

func newWorker(config *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, gasFloor, gasCeil uint64, cch core.CrossChainHelper) *worker {
	worker := &worker{
		config:         config,
		engine:         engine,
		eth:            eth,
		mux:            mux,
		gasFloor:       gasFloor,
		gasCeil:        gasCeil,
		txCh:           make(chan core.TxPreEvent, txChanSize),
		chainHeadCh:    make(chan core.ChainHeadEvent, chainHeadChanSize),
		chainSideCh:    make(chan core.ChainSideEvent, chainSideChanSize),
		resultCh:       make(chan *Result, resultQueueSize),
		exitCh:         make(chan struct{}),
		chain:          eth.BlockChain(),
		proc:           eth.BlockChain().Validator(),
		possibleUncles: make(map[common.Hash]*types.Block),
		agents:         make(map[Agent]struct{}),
		unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth, config.ChainLogger),
		logger:         config.ChainLogger,
		cch:            cch,
	}

	worker.txSub = eth.TxPool().SubscribeTxPreEvent(worker.txCh)

	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainSideSub = eth.BlockChain().SubscribeChainSideEvent(worker.chainSideCh)

	go worker.mainLoop()

	go worker.resultLoop()

	return worker
}

func (self *worker) setCoinbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.coinbase = addr
}

func (self *worker) setExtra(extra []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.extra = extra
}

func (self *worker) pending() (*types.Block, *state.StateDB) {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		), self.current.state.Copy()
	}
	return self.current.Block, self.current.state.Copy()
}

func (self *worker) pendingBlock() *types.Block {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		)
	}
	return self.current.Block
}

func (self *worker) start() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.StoreInt32(&self.mining, 1)

	if neatcon, ok := self.engine.(consensus.NeatCon); ok {
		err := neatcon.Start(self.chain, self.chain.CurrentBlock, self.chain.HasBadBlock)
		if err != nil {
			self.logger.Error("Starting NeatCon failed", "err", err)
		}
	}

	for agent := range self.agents {
		agent.Start()
	}
}

func (self *worker) stop() {
	self.wg.Wait()

	self.mu.Lock()
	defer self.mu.Unlock()
	if atomic.LoadInt32(&self.mining) == 1 {
		for agent := range self.agents {
			agent.Stop()
		}
	}

	if stoppableEngine, ok := self.engine.(consensus.EngineStartStop); ok {
		engineStopErr := stoppableEngine.Stop()
		if engineStopErr != nil {
			self.logger.Error("Stop Engine failed.", "err", engineStopErr)
		} else {
			self.logger.Info("Stop Engine Success.")
		}
	}

	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.atWork, 0)
}

func (self *worker) isRunning() bool {
	return atomic.LoadInt32(&self.mining) == 1
}

func (self *worker) close() {
	close(self.exitCh)
}

func (self *worker) register(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.agents[agent] = struct{}{}
	agent.SetReturnCh(self.resultCh)
}

func (self *worker) unregister(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	delete(self.agents, agent)
	agent.Stop()
}

func (self *worker) mainLoop() {
	defer self.txSub.Unsubscribe()
	defer self.chainHeadSub.Unsubscribe()
	defer self.chainSideSub.Unsubscribe()

	for {

		select {

		case ev := <-self.chainHeadCh:
			if h, ok := self.engine.(consensus.Handler); ok {
				h.NewChainHead(ev.Block)
			}
			self.commitNewWork()

		case ev := <-self.chainSideCh:
			self.uncleMu.Lock()
			self.possibleUncles[ev.Block.Hash()] = ev.Block
			self.uncleMu.Unlock()

		case ev := <-self.txCh:

			if !self.isRunning() && self.current != nil {
				self.currentMu.Lock()
				acc, _ := types.Sender(self.current.signer, ev.Tx)
				txs := map[common.Address]types.Transactions{acc: {ev.Tx}}
				txset := types.NewTransactionsByPriceAndNonce(self.current.signer, txs)

				self.commitTransactionsEx(txset, self.coinbase, big.NewInt(0), self.cch)
				self.currentMu.Unlock()
			}

		case <-self.exitCh:
			return
		case <-self.txSub.Err():
			return
		case <-self.chainHeadSub.Err():
			return
		case <-self.chainSideSub.Err():
			return
		}
	}
}

func (self *worker) resultLoop() {
	for {
		mustCommitNewWork := true
		select {
		case result := <-self.resultCh:
			atomic.AddInt32(&self.atWork, -1)

			if result == nil {
				continue
			}

			var block *types.Block
			var receipts types.Receipts
			var state *state.StateDB
			var ops *types.PendingOps

			if result.Work != nil {
				block = result.Block
				hash := block.Hash()
				work := result.Work

				for i, receipt := range work.receipts {

					receipt.BlockHash = hash
					receipt.BlockNumber = block.Number()
					receipt.TransactionIndex = uint(i)

					for _, l := range receipt.Logs {
						l.BlockHash = hash
					}
				}
				for _, log := range work.state.Logs() {
					log.BlockHash = hash
				}
				receipts = work.receipts
				state = work.state
				ops = work.ops
			} else if result.Intermediate != nil {
				block = result.Intermediate.Block

				for i, receipt := range result.Intermediate.Receipts {

					receipt.BlockHash = block.Hash()
					receipt.BlockNumber = block.Number()
					receipt.TransactionIndex = uint(i)
				}

				receipts = result.Intermediate.Receipts
				state = result.Intermediate.State
				ops = result.Intermediate.Ops
			} else {
				continue
			}

			self.chain.MuLock()

			stat, err := self.chain.WriteBlockWithState(block, receipts, state)
			if err != nil {
				self.logger.Error("Failed writing block to chain", "err", err)
				self.chain.MuUnLock()
				continue
			}

			for _, op := range ops.Ops() {
				if err := core.ApplyOp(op, self.chain, self.cch); err != nil {
					log.Error("Failed executing op", op, "err", err)
				}
			}

			if stat == core.CanonStatTy {

				mustCommitNewWork = false
			}

			self.mux.Post(core.NewMinedBlockEvent{Block: block})
			var (
				events []interface{}
				logs   = state.Logs()
			)
			events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
			if stat == core.CanonStatTy {
				events = append(events, core.ChainHeadEvent{Block: block})
			}

			self.chain.MuUnLock()

			self.chain.PostChainEvents(events, logs)

			self.unconfirmed.Insert(block.NumberU64(), block.Hash())

			if mustCommitNewWork {
				self.commitNewWork()
			}
		case <-self.exitCh:
			return
		}
	}
}

func (self *worker) push(work *Work) {
	if atomic.LoadInt32(&self.mining) != 1 {
		return
	}
	for agent := range self.agents {
		atomic.AddInt32(&self.atWork, 1)
		if ch := agent.Work(); ch != nil {
			ch <- work
		}
	}
}

func (self *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := self.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &Work{
		signer:    types.NewEIP155Signer(self.config.ChainId),
		state:     state,
		ancestors: set.New(),
		family:    set.New(),
		uncles:    set.New(),
		header:    header,
		ops:       new(types.PendingOps),
		createdAt: time.Now(),
		logger:    self.logger,
	}

	for _, ancestor := range self.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			work.family.Add(uncle.Hash())
		}
		work.family.Add(ancestor.Hash())
		work.ancestors.Add(ancestor.Hash())
	}

	work.tcount = 0
	self.current = work
	return nil
}

func (self *worker) commitNewWork() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.uncleMu.Lock()
	defer self.uncleMu.Unlock()
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	tstart := time.Now()
	parent := self.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time() >= uint64(tstamp) {
		tstamp = int64(parent.Time() + 1)
	}

	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		self.logger.Info("Mining too far in the future", "suppose but not wait", common.PrettyDuration(wait))

	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent, self.gasFloor, self.gasCeil),
		Extra:      self.extra,
		Time:       big.NewInt(tstamp),
	}

	if self.isRunning() {
		if self.coinbase == (common.Address{}) {
			log.Error("Refusing to mine without coinbase")
			return
		}
		header.Coinbase = self.coinbase
	}
	if err := self.engine.Prepare(self.chain, header); err != nil {
		self.logger.Error("Failed to prepare header for mining", "err", err)
		return
	}

	err := self.makeCurrent(parent, header)
	if err != nil {
		self.logger.Error("Failed to create mining context", "err", err)
		return
	}

	work := self.current

	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		self.logger.Error("Failed to fetch pending transactions", "err", err)
		return
	}

	totalUsedMoney := big.NewInt(0)
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)

	rmTxs := self.commitTransactionsEx(txs, self.coinbase, totalUsedMoney, self.cch)

	if len(rmTxs) > 0 {
		self.eth.TxPool().RemoveTxs(rmTxs)
	}

	var (
		uncles    []*types.Header
		badUncles []common.Hash
	)
	for hash, uncle := range self.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := self.commitUncle(work, uncle.Header()); err != nil {
			self.logger.Trace("Bad uncle found and will be removed", "hash", hash)
			self.logger.Trace(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			self.logger.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(self.possibleUncles, hash)
	}

	if work.Block, err = self.engine.Finalize(self.chain, header, work.state, work.txs, totalUsedMoney, uncles, work.receipts, work.ops); err != nil {
		self.logger.Error("Failed to finalize block for sealing", "err", err)
		return
	}

	if self.isRunning() {

		self.unconfirmed.Shift(work.Block.NumberU64() - 1)
	}
	self.push(work)
}

func (self *worker) commitUncle(work *Work, uncle *types.Header) error {
	hash := uncle.Hash()
	if work.uncles.Has(hash) {
		return fmt.Errorf("uncle not unique")
	}
	if !work.ancestors.Has(uncle.ParentHash) {
		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
	}
	if work.family.Has(hash) {
		return fmt.Errorf("uncle already in family (%x)", hash)
	}
	work.uncles.Add(uncle.Hash())
	return nil
}

func (self *worker) commitTransactionsEx(txs *types.TransactionsByPriceAndNonce, coinbase common.Address, totalUsedMoney *big.Int, cch core.CrossChainHelper) (rmTxs types.Transactions) {

	gp := new(core.GasPool).AddGas(self.current.header.GasLimit)

	var coalescedLogs []*types.Log

	for {

		if gp.Gas() < params.TxGas {
			self.logger.Trace("Not enough gas for further transactions", "have", gp, "want", params.TxGas)
			break
		}

		tx := txs.Peek()
		if tx == nil {
			break
		}

		from, _ := types.Sender(self.current.signer, tx)

		if tx.Protected() && !self.config.IsEIP155(self.current.header.Number) {
			self.logger.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", self.config.EIP155Block)

			txs.Pop()
			continue
		}

		self.current.state.Prepare(tx.Hash(), common.Hash{}, self.current.tcount)

		logs, err := self.commitTransactionEx(tx, coinbase, gp, totalUsedMoney, cch)
		switch err {
		case core.ErrGasLimitReached:

			self.logger.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:

			self.logger.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:

			self.logger.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case core.ErrInvalidTx4:

			rmTxs = append(rmTxs, tx)
			self.logger.Trace("Invalid Tx4, this tx will be removed", "hash", tx.Hash())
			txs.Shift()

		case nil:

			coalescedLogs = append(coalescedLogs, logs...)
			self.current.tcount++
			txs.Shift()

		default:

			self.logger.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if len(coalescedLogs) > 0 || self.current.tcount > 0 {

		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go func(logs []*types.Log, tcount int) {
			if len(logs) > 0 {
				self.mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				self.mux.Post(core.PendingStateEvent{})
			}
		}(cpy, self.current.tcount)
	}
	return
}

func (self *worker) commitTransactionEx(tx *types.Transaction, coinbase common.Address, gp *core.GasPool, totalUsedMoney *big.Int, cch core.CrossChainHelper) ([]*types.Log, error) {
	snap := self.current.state.Snapshot()

	receipt, err := core.ApplyTransactionEx(self.config, self.chain, nil, gp, self.current.state, self.current.ops, self.current.header, tx, &self.current.header.GasUsed, totalUsedMoney, vm.Config{}, cch, true)
	if err != nil {
		self.current.state.RevertToSnapshot(snap)
		return nil, err
	}

	self.current.txs = append(self.current.txs, tx)
	self.current.receipts = append(self.current.receipts, receipt)

	return receipt.Logs, nil
}
