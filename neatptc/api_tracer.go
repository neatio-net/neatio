package neatptc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"runtime"
	"sync"
	"time"

	"github.com/neatio-net/neatio/chain/core"
	"github.com/neatio-net/neatio/chain/core/rawdb"
	"github.com/neatio-net/neatio/chain/core/state"
	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/chain/core/vm"
	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/chain/trie"
	"github.com/neatio-net/neatio/internal/neatapi"
	"github.com/neatio-net/neatio/neatptc/tracers"
	"github.com/neatio-net/neatio/network/rpc"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/common/hexutil"
	"github.com/neatio-net/neatio/utilities/rlp"
)

const (
	defaultTraceTimeout = 5 * time.Second

	defaultTraceReexec = uint64(128)
)

type TraceConfig struct {
	*vm.LogConfig
	Tracer  *string
	Timeout *string
	Reexec  *uint64
}

type StdTraceConfig struct {
	*vm.LogConfig
	Reexec *uint64
	TxHash common.Hash
}

type txTraceResult struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type blockTraceTask struct {
	statedb *state.StateDB
	block   *types.Block
	rootref common.Hash
	results []*txTraceResult
}

type blockTraceResult struct {
	Block  hexutil.Uint64   `json:"block"`
	Hash   common.Hash      `json:"hash"`
	Traces []*txTraceResult `json:"traces"`
}

type txTraceTask struct {
	statedb *state.StateDB
	index   int
}

func (api *PrivateDebugAPI) TraceChain(ctx context.Context, start, end rpc.BlockNumber, config *TraceConfig) (*rpc.Subscription, error) {

	var from, to *types.Block

	switch start {
	case rpc.PendingBlockNumber:
		from = api.eth.miner.PendingBlock()
	case rpc.LatestBlockNumber:
		from = api.eth.blockchain.CurrentBlock()
	default:
		from = api.eth.blockchain.GetBlockByNumber(uint64(start))
	}
	switch end {
	case rpc.PendingBlockNumber:
		to = api.eth.miner.PendingBlock()
	case rpc.LatestBlockNumber:
		to = api.eth.blockchain.CurrentBlock()
	default:
		to = api.eth.blockchain.GetBlockByNumber(uint64(end))
	}

	if from == nil {
		return nil, fmt.Errorf("starting block #%d not found", start)
	}
	if to == nil {
		return nil, fmt.Errorf("end block #%d not found", end)
	}
	if from.Number().Cmp(to.Number()) >= 0 {
		return nil, fmt.Errorf("end block (#%d) needs to come after start block (#%d)", end, start)
	}
	return api.traceChain(ctx, from, to, config)
}

func (api *PrivateDebugAPI) traceChain(ctx context.Context, start, end *types.Block, config *TraceConfig) (*rpc.Subscription, error) {

	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	origin := start.NumberU64()
	database := state.NewDatabaseWithCache(api.eth.ChainDb(), 16)

	if number := start.NumberU64(); number > 0 {
		start = api.eth.blockchain.GetBlock(start.ParentHash(), start.NumberU64()-1)
		if start == nil {
			return nil, fmt.Errorf("parent block #%d not found", number-1)
		}
	}
	statedb, err := state.New(start.Root(), database)
	if err != nil {

		reexec := defaultTraceReexec
		if config != nil && config.Reexec != nil {
			reexec = *config.Reexec
		}

		for i := uint64(0); i < reexec; i++ {
			start = api.eth.blockchain.GetBlock(start.ParentHash(), start.NumberU64()-1)
			if start == nil {
				break
			}
			if statedb, err = state.New(start.Root(), database); err == nil {
				break
			}
		}

		if err != nil {
			switch err.(type) {
			case *trie.MissingNodeError:
				return nil, errors.New("required historical state unavailable")
			default:
				return nil, err
			}
		}
	}

	blocks := int(end.NumberU64() - origin)

	threads := runtime.NumCPU()
	if threads > blocks {
		threads = blocks
	}
	var (
		pend    = new(sync.WaitGroup)
		tasks   = make(chan *blockTraceTask, threads)
		results = make(chan *blockTraceTask, threads)
	)
	for th := 0; th < threads; th++ {
		pend.Add(1)
		go func() {
			defer pend.Done()

			for task := range tasks {
				signer := types.MakeSigner(api.eth.blockchain.Config(), task.block.Number())

				for i, tx := range task.block.Transactions() {
					msg, _ := tx.AsMessage(signer)
					vmctx := core.NewEVMContext(msg, task.block.Header(), api.eth.blockchain, nil)

					res, err := api.traceTx(ctx, msg, vmctx, task.statedb, config)
					if err != nil {
						task.results[i] = &txTraceResult{Error: err.Error()}
						log.Warn("Tracing failed", "hash", tx.Hash(), "block", task.block.NumberU64(), "err", err)
						break
					}

					task.statedb.Finalise(api.eth.blockchain.Config().IsEIP158(task.block.Number()))
					task.results[i] = &txTraceResult{Result: res}
				}

				select {
				case results <- task:
				case <-notifier.Closed():
					return
				}
			}
		}()
	}

	begin := time.Now()

	go func() {
		var (
			logged time.Time
			number uint64
			traced uint64
			failed error
			proot  common.Hash
		)

		defer func() {
			close(tasks)
			pend.Wait()

			switch {
			case failed != nil:
				log.Warn("Chain tracing failed", "start", start.NumberU64(), "end", end.NumberU64(), "transactions", traced, "elapsed", time.Since(begin), "err", failed)
			case number < end.NumberU64():
				log.Warn("Chain tracing aborted", "start", start.NumberU64(), "end", end.NumberU64(), "abort", number, "transactions", traced, "elapsed", time.Since(begin))
			default:
				log.Info("Chain tracing finished", "start", start.NumberU64(), "end", end.NumberU64(), "transactions", traced, "elapsed", time.Since(begin))
			}
			close(results)
		}()

		for number = start.NumberU64() + 1; number <= end.NumberU64(); number++ {

			select {
			case <-notifier.Closed():
				return
			default:
			}

			if time.Since(logged) > 8*time.Second {
				if number > origin {
					nodes, imgs := database.TrieDB().Size()
					log.Info("Tracing chain segment", "start", origin, "end", end.NumberU64(), "current", number, "transactions", traced, "elapsed", time.Since(begin), "memory", nodes+imgs)
				} else {
					log.Info("Preparing state for chain trace", "block", number, "start", origin, "elapsed", time.Since(begin))
				}
				logged = time.Now()
			}

			block := api.eth.blockchain.GetBlockByNumber(number)
			if block == nil {
				failed = fmt.Errorf("block #%d not found", number)
				break
			}

			if number > origin {
				txs := block.Transactions()

				select {
				case tasks <- &blockTraceTask{statedb: statedb.Copy(), block: block, rootref: proot, results: make([]*txTraceResult, len(txs))}:
				case <-notifier.Closed():
					return
				}
				traced += uint64(len(txs))
			}

			_, _, _, _, err := api.eth.blockchain.Processor().Process(block, statedb, vm.Config{})
			if err != nil {
				failed = err
				break
			}

			root, err := statedb.Commit(api.eth.blockchain.Config().IsEIP158(block.Number()))
			if err != nil {
				failed = err
				break
			}
			if err := statedb.Reset(root); err != nil {
				failed = err
				break
			}

			database.TrieDB().Reference(root, common.Hash{})
			if number >= origin {
				database.TrieDB().Reference(root, common.Hash{})
			}

			if proot != (common.Hash{}) {
				database.TrieDB().Dereference(proot)
			}
			proot = root

		}
	}()

	go func() {
		var (
			done = make(map[uint64]*blockTraceResult)
			next = origin + 1
		)
		for res := range results {

			result := &blockTraceResult{
				Block:  hexutil.Uint64(res.block.NumberU64()),
				Hash:   res.block.Hash(),
				Traces: res.results,
			}
			done[uint64(result.Block)] = result

			database.TrieDB().Dereference(res.rootref)

			for result, ok := done[next]; ok; result, ok = done[next] {
				if len(result.Traces) > 0 || next == end.NumberU64() {
					notifier.Notify(sub.ID, result)
				}
				delete(done, next)
				next++
			}
		}
	}()
	return sub, nil
}

func (api *PrivateDebugAPI) TraceBlockByNumber(ctx context.Context, number rpc.BlockNumber, config *TraceConfig) ([]*txTraceResult, error) {

	var block *types.Block

	switch number {
	case rpc.PendingBlockNumber:
		block = api.eth.miner.PendingBlock()
	case rpc.LatestBlockNumber:
		block = api.eth.blockchain.CurrentBlock()
	default:
		block = api.eth.blockchain.GetBlockByNumber(uint64(number))
	}

	if block == nil {
		return nil, fmt.Errorf("block #%d not found", number)
	}
	return api.traceBlock(ctx, block, config)
}

func (api *PrivateDebugAPI) TraceBlockByHash(ctx context.Context, hash common.Hash, config *TraceConfig) ([]*txTraceResult, error) {
	block := api.eth.blockchain.GetBlockByHash(hash)
	if block == nil {
		return nil, fmt.Errorf("block #%x not found", hash)
	}
	return api.traceBlock(ctx, block, config)
}

func (api *PrivateDebugAPI) TraceBlock(ctx context.Context, blob []byte, config *TraceConfig) ([]*txTraceResult, error) {
	block := new(types.Block)
	if err := rlp.Decode(bytes.NewReader(blob), block); err != nil {
		return nil, fmt.Errorf("could not decode block: %v", err)
	}
	return api.traceBlock(ctx, block, config)
}

func (api *PrivateDebugAPI) TraceBlockFromFile(ctx context.Context, file string, config *TraceConfig) ([]*txTraceResult, error) {
	blob, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("could not read file: %v", err)
	}
	return api.TraceBlock(ctx, blob, config)
}

func (api *PrivateDebugAPI) traceBlock(ctx context.Context, block *types.Block, config *TraceConfig) ([]*txTraceResult, error) {

	if err := api.eth.engine.VerifyHeader(api.eth.blockchain, block.Header(), true); err != nil {
		return nil, err
	}
	parent := api.eth.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		return nil, fmt.Errorf("parent #%x not found", block.ParentHash())
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	statedb, err := api.computeStateDB(parent, reexec)
	if err != nil {
		return nil, err
	}

	var (
		signer = types.MakeSigner(api.eth.blockchain.Config(), block.Number())

		txs     = block.Transactions()
		results = make([]*txTraceResult, len(txs))

		pend = new(sync.WaitGroup)
		jobs = make(chan *txTraceTask, len(txs))
	)
	threads := runtime.NumCPU()
	if threads > len(txs) {
		threads = len(txs)
	}
	for th := 0; th < threads; th++ {
		pend.Add(1)
		go func() {
			defer pend.Done()

			for task := range jobs {
				msg, _ := txs[task.index].AsMessage(signer)
				vmctx := core.NewEVMContext(msg, block.Header(), api.eth.blockchain, nil)

				res, err := api.traceTx(ctx, msg, vmctx, task.statedb, config)
				if err != nil {
					results[task.index] = &txTraceResult{Error: err.Error()}
					continue
				}
				results[task.index] = &txTraceResult{Result: res}
			}
		}()
	}

	var failed error
	for i, tx := range txs {

		jobs <- &txTraceTask{statedb: statedb.Copy(), index: i}

		msg, _ := tx.AsMessage(signer)
		vmctx := core.NewEVMContext(msg, block.Header(), api.eth.blockchain, nil)

		vmenv := vm.NewEVM(vmctx, statedb, api.eth.blockchain.Config(), vm.Config{})
		if _, _, err := core.ApplyMessageEx(vmenv, msg, new(core.GasPool).AddGas(msg.Gas())); err != nil {
			failed = err
			break
		}

		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	close(jobs)
	pend.Wait()

	if failed != nil {
		return nil, failed
	}
	return results, nil
}

func (api *PrivateDebugAPI) computeStateDB(block *types.Block, reexec uint64) (*state.StateDB, error) {

	statedb, err := api.eth.blockchain.StateAt(block.Root())
	if err == nil {
		return statedb, nil
	}

	origin := block.NumberU64()
	database := state.NewDatabaseWithCache(api.eth.ChainDb(), 16)

	for i := uint64(0); i < reexec; i++ {
		block = api.eth.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1)
		if block == nil {
			break
		}
		if statedb, err = state.New(block.Root(), database); err == nil {
			break
		}
	}
	if err != nil {
		switch err.(type) {
		case *trie.MissingNodeError:
			return nil, fmt.Errorf("required historical state unavailable (reexec=%d)", reexec)
		default:
			return nil, err
		}
	}

	var (
		start  = time.Now()
		logged time.Time
		proot  common.Hash
	)
	for block.NumberU64() < origin {

		if time.Since(logged) > 8*time.Second {
			log.Info("Regenerating historical state", "block", block.NumberU64()+1, "target", origin, "remaining", origin-block.NumberU64()-1, "elapsed", time.Since(start))
			logged = time.Now()
		}

		if block = api.eth.blockchain.GetBlockByNumber(block.NumberU64() + 1); block == nil {
			return nil, fmt.Errorf("block #%d not found", block.NumberU64()+1)
		}
		_, _, _, _, err := api.eth.blockchain.Processor().Process(block, statedb, vm.Config{})
		if err != nil {
			return nil, fmt.Errorf("processing block %d failed: %v", block.NumberU64(), err)
		}

		root, err := statedb.Commit(api.eth.blockchain.Config().IsEIP158(block.Number()))
		if err != nil {
			return nil, err
		}
		if err := statedb.Reset(root); err != nil {
			return nil, fmt.Errorf("state reset after block %d failed: %v", block.NumberU64(), err)
		}
		database.TrieDB().Reference(root, common.Hash{})
		if proot != (common.Hash{}) {
			database.TrieDB().Dereference(proot)
		}
		proot = root
	}
	nodes, imgs := database.TrieDB().Size()
	log.Info("Historical state regenerated", "block", block.NumberU64(), "elapsed", time.Since(start), "nodes", nodes, "preimages", imgs)
	return statedb, nil
}

func (api *PrivateDebugAPI) TraceTransaction(ctx context.Context, hash common.Hash, config *TraceConfig) (interface{}, error) {

	tx, blockHash, _, index := rawdb.ReadTransaction(api.eth.ChainDb(), hash)
	if tx == nil {
		return nil, fmt.Errorf("transaction #%x not found", hash)
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	msg, vmctx, statedb, err := api.computeTxEnv(blockHash, int(index), reexec)
	if err != nil {
		return nil, err
	}

	return api.traceTx(ctx, msg, vmctx, statedb, config)
}

func (api *PrivateDebugAPI) traceTx(ctx context.Context, message core.Message, vmctx vm.Context, statedb *state.StateDB, config *TraceConfig) (interface{}, error) {

	var (
		tracer vm.Tracer
		err    error
	)
	switch {
	case config != nil && config.Tracer != nil:

		timeout := defaultTraceTimeout
		if config.Timeout != nil {
			if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
				return nil, err
			}
		}

		if tracer, err = tracers.New(*config.Tracer); err != nil {
			return nil, err
		}

		deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
		go func() {
			<-deadlineCtx.Done()
			tracer.(*tracers.Tracer).Stop(errors.New("execution timeout"))
		}()
		defer cancel()

	case config == nil:
		tracer = vm.NewStructLogger(nil)

	default:
		tracer = vm.NewStructLogger(config.LogConfig)
	}

	vmenv := vm.NewEVM(vmctx, statedb, api.eth.blockchain.Config(), vm.Config{Debug: true, Tracer: tracer})

	result, _, err := core.ApplyMessageEx(vmenv, message, new(core.GasPool).AddGas(message.Gas()))
	if err != nil {
		return nil, fmt.Errorf("tracing failed: %v", err)
	}

	switch tracer := tracer.(type) {
	case *vm.StructLogger:
		return &neatapi.ExecutionResult{
			Gas:         result.UsedGas,
			Failed:      result.Failed(),
			ReturnValue: fmt.Sprintf("%x", result.Revert()),
			StructLogs:  neatapi.FormatLogs(tracer.StructLogs()),
		}, nil

	case *tracers.Tracer:
		return tracer.GetResult()

	default:
		panic(fmt.Sprintf("bad tracer type %T", tracer))
	}
}

func (api *PrivateDebugAPI) computeTxEnv(blockHash common.Hash, txIndex int, reexec uint64) (core.Message, vm.Context, *state.StateDB, error) {

	block := api.eth.blockchain.GetBlockByHash(blockHash)
	if block == nil {
		return nil, vm.Context{}, nil, fmt.Errorf("block %#x not found", blockHash)
	}
	parent := api.eth.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		return nil, vm.Context{}, nil, fmt.Errorf("parent %#x not found", block.ParentHash())
	}
	statedb, err := api.computeStateDB(parent, reexec)
	if err != nil {
		return nil, vm.Context{}, nil, err
	}

	if txIndex == 0 && len(block.Transactions()) == 0 {
		return nil, vm.Context{}, statedb, nil
	}

	signer := types.MakeSigner(api.eth.blockchain.Config(), block.Number())

	for idx, tx := range block.Transactions() {

		msg, _ := tx.AsMessage(signer)
		context := core.NewEVMContext(msg, block.Header(), api.eth.blockchain, nil)
		if idx == txIndex {
			return msg, context, statedb, nil
		}

		vmenv := vm.NewEVM(context, statedb, api.eth.blockchain.Config(), vm.Config{})
		if _, _, err := core.ApplyMessageEx(vmenv, msg, new(core.GasPool).AddGas(tx.Gas())); err != nil {
			return nil, vm.Context{}, nil, fmt.Errorf("transaction %#x failed: %v", tx.Hash(), err)
		}

		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	return nil, vm.Context{}, nil, fmt.Errorf("transaction index %d out of range for block %#x", txIndex, blockHash)
}
