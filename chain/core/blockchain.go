package core

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neatio-net/neatio/chain/core/rawdb"

	lru "github.com/hashicorp/golang-lru"
	"github.com/neatio-net/neatio/chain/consensus"
	"github.com/neatio-net/neatio/chain/core/state"
	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/chain/core/vm"
	"github.com/neatio-net/neatio/chain/log"
	"github.com/neatio-net/neatio/chain/trie"
	"github.com/neatio-net/neatio/neatdb"
	"github.com/neatio-net/neatio/params"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/common/mclock"
	"github.com/neatio-net/neatio/utilities/common/prque"
	"github.com/neatio-net/neatio/utilities/crypto"
	"github.com/neatio-net/neatio/utilities/event"
	"github.com/neatio-net/neatio/utilities/metrics"
	"github.com/neatio-net/neatio/utilities/rlp"
)

var (
	blockInsertTimer = metrics.NewRegisteredTimer("chain/inserts", nil)

	ErrNoGenesis = errors.New("Genesis not found in chain")
)

const (
	bodyCacheLimit      = 256
	blockCacheLimit     = 256
	receiptsCacheLimit  = 32
	maxFutureBlocks     = 256
	maxTimeFutureBlocks = 30
	badBlockLimit       = 10
	triesInMemory       = 128

	BlockChainVersion = 3

	TimeForBanned  = 2 * time.Hour
	BannedDuration = 24 * time.Hour
)

type CacheConfig struct {
	TrieCleanLimit int

	TrieDirtyLimit    int
	TrieDirtyDisabled bool
	TrieTimeLimit     time.Duration
}

type BlockChain struct {
	chainConfig *params.ChainConfig
	cacheConfig *CacheConfig

	db     neatdb.Database
	triegc *prque.Prque
	gcproc time.Duration

	hc                  *HeaderChain
	rmLogsFeed          event.Feed
	chainFeed           event.Feed
	chainSideFeed       event.Feed
	chainHeadFeed       event.Feed
	logsFeed            event.Feed
	createSideChainFeed event.Feed
	startMiningFeed     event.Feed
	stopMiningFeed      event.Feed

	scope        event.SubscriptionScope
	genesisBlock *types.Block

	chainmu sync.RWMutex

	currentBlock     atomic.Value
	currentFastBlock atomic.Value

	stateCache    state.Database
	bodyCache     *lru.Cache
	bodyRLPCache  *lru.Cache
	receiptsCache *lru.Cache
	blockCache    *lru.Cache
	futureBlocks  *lru.Cache

	quit    chan struct{}
	running int32

	procInterrupt int32
	wg            sync.WaitGroup

	engine    consensus.Engine
	validator Validator
	processor Processor
	vmConfig  vm.Config

	badBlocks *lru.Cache

	cch    CrossChainHelper
	logger log.Logger
}

func NewBlockChain(db neatdb.Database, cacheConfig *CacheConfig, chainConfig *params.ChainConfig, engine consensus.Engine, vmConfig vm.Config, cch CrossChainHelper) (*BlockChain, error) {
	if cacheConfig == nil {
		cacheConfig = &CacheConfig{
			TrieCleanLimit: 256,
			TrieDirtyLimit: 256,
			TrieTimeLimit:  5 * time.Minute,
		}
	}
	bodyCache, _ := lru.New(bodyCacheLimit)
	bodyRLPCache, _ := lru.New(bodyCacheLimit)
	receiptsCache, _ := lru.New(receiptsCacheLimit)
	blockCache, _ := lru.New(blockCacheLimit)
	futureBlocks, _ := lru.New(maxFutureBlocks)
	badBlocks, _ := lru.New(badBlockLimit)

	bc := &BlockChain{
		chainConfig:   chainConfig,
		cacheConfig:   cacheConfig,
		db:            db,
		triegc:        prque.New(nil),
		stateCache:    state.NewDatabaseWithCache(db, cacheConfig.TrieCleanLimit),
		quit:          make(chan struct{}),
		bodyCache:     bodyCache,
		bodyRLPCache:  bodyRLPCache,
		receiptsCache: receiptsCache,
		blockCache:    blockCache,
		futureBlocks:  futureBlocks,
		engine:        engine,
		vmConfig:      vmConfig,
		badBlocks:     badBlocks,
		cch:           cch,
		logger:        chainConfig.ChainLogger,
	}
	bc.validator = NewBlockValidator(chainConfig, bc, engine)
	bc.processor = NewStateProcessor(chainConfig, bc, engine, cch)

	var err error
	bc.hc, err = NewHeaderChain(db, chainConfig, engine, bc.getProcInterrupt)
	if err != nil {
		return nil, err
	}
	bc.genesisBlock = bc.GetBlockByNumber(0)
	if bc.genesisBlock == nil {
		return nil, ErrNoGenesis
	}
	if err := bc.loadLastState(); err != nil {
		return nil, err
	}

	for hash := range BadHashes {
		if header := bc.GetHeaderByHash(hash); header != nil {

			headerByNumber := bc.GetHeaderByNumber(header.Number.Uint64())

			if headerByNumber != nil && headerByNumber.Hash() == header.Hash() {
				bc.logger.Error("Found bad hash, rewinding chain", "number", header.Number, "hash", header.ParentHash)
				bc.SetHead(header.Number.Uint64() - 1)
				bc.logger.Error("Chain rewind was successful, resuming normal operation")
			}
		}
	}

	go bc.update()
	return bc, nil
}

func (bc *BlockChain) getProcInterrupt() bool {
	return atomic.LoadInt32(&bc.procInterrupt) == 1
}

func (bc *BlockChain) loadLastState() error {

	head := rawdb.ReadHeadBlockHash(bc.db)
	if head == (common.Hash{}) {

		bc.logger.Warn("Empty database, resetting chain")
		return bc.Reset()
	}

	currentBlock := bc.GetBlockByHash(head)
	if currentBlock == nil {

		bc.logger.Warn("Head block missing, resetting chain", "hash", head)
		return bc.Reset()
	}

	if _, err := state.New(currentBlock.Root(), bc.stateCache); err != nil {

		bc.logger.Warn("Head state missing, repairing chain", "number", currentBlock.Number(), "hash", currentBlock.Hash(), "err", err)
		if err := bc.repair(&currentBlock); err != nil {
			return err
		}
	}

	bc.currentBlock.Store(currentBlock)

	currentHeader := currentBlock.Header()
	if head := rawdb.ReadHeadHeaderHash(bc.db); head != (common.Hash{}) {
		if header := bc.GetHeaderByHash(head); header != nil {
			currentHeader = header
		}
	}
	bc.hc.SetCurrentHeader(currentHeader)

	bc.currentFastBlock.Store(currentBlock)
	if head := rawdb.ReadHeadFastBlockHash(bc.db); head != (common.Hash{}) {
		if block := bc.GetBlockByHash(head); block != nil {
			bc.currentFastBlock.Store(block)
		}
	}

	bc.logger.Info("Local chain block height is:", "Block", currentHeader.Number, "Hash", currentHeader.Hash())

	return nil
}

func (bc *BlockChain) SetHead(head uint64) error {
	bc.logger.Warn("Rewinding blockchain", "target", head)

	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	delFn := func(db neatdb.Writer, hash common.Hash, num uint64) {
		rawdb.DeleteBody(db, hash, num)
	}
	bc.hc.SetHead(head, delFn)
	currentHeader := bc.hc.CurrentHeader()

	bc.bodyCache.Purge()
	bc.bodyRLPCache.Purge()
	bc.receiptsCache.Purge()
	bc.blockCache.Purge()
	bc.futureBlocks.Purge()

	if currentBlock := bc.CurrentBlock(); currentBlock != nil && currentHeader.Number.Uint64() < currentBlock.NumberU64() {
		bc.currentBlock.Store(bc.GetBlock(currentHeader.Hash(), currentHeader.Number.Uint64()))
	}
	if currentBlock := bc.CurrentBlock(); currentBlock != nil {
		if _, err := state.New(currentBlock.Root(), bc.stateCache); err != nil {

			bc.currentBlock.Store(bc.genesisBlock)
		}
	}

	if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock != nil && currentHeader.Number.Uint64() < currentFastBlock.NumberU64() {
		bc.currentFastBlock.Store(bc.GetBlock(currentHeader.Hash(), currentHeader.Number.Uint64()))
	}

	if currentBlock := bc.CurrentBlock(); currentBlock == nil {
		bc.currentBlock.Store(bc.genesisBlock)
	}
	if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock == nil {
		bc.currentFastBlock.Store(bc.genesisBlock)
	}
	currentBlock := bc.CurrentBlock()
	currentFastBlock := bc.CurrentFastBlock()

	rawdb.WriteHeadBlockHash(bc.db, currentBlock.Hash())
	rawdb.WriteHeadFastBlockHash(bc.db, currentFastBlock.Hash())

	return bc.loadLastState()
}

func (bc *BlockChain) FastSyncCommitHead(hash common.Hash) error {

	block := bc.GetBlockByHash(hash)
	if block == nil {
		return fmt.Errorf("non existent block [%x…]", hash[:4])
	}
	if _, err := trie.NewSecure(block.Root(), bc.stateCache.TrieDB()); err != nil {
		return err
	}

	bc.chainmu.Lock()
	bc.currentBlock.Store(block)
	bc.chainmu.Unlock()

	bc.logger.Info("Committed new head block", "number", block.Number(), "hash", hash)
	return nil
}

func (bc *BlockChain) GasLimit() uint64 {
	return bc.CurrentBlock().GasLimit()
}

func (bc *BlockChain) CurrentBlock() *types.Block {
	return bc.currentBlock.Load().(*types.Block)
}

func (bc *BlockChain) CurrentFastBlock() *types.Block {
	return bc.currentFastBlock.Load().(*types.Block)
}

func (bc *BlockChain) Validator() Validator {
	return bc.validator
}

func (bc *BlockChain) Processor() Processor {
	return bc.processor
}

func (bc *BlockChain) State() (*state.StateDB, error) {
	return bc.StateAt(bc.CurrentBlock().Root())
}

func (bc *BlockChain) StateAt(root common.Hash) (*state.StateDB, error) {
	return state.New(root, bc.stateCache)
}

func (bc *BlockChain) StateCache() state.Database {
	return bc.stateCache
}

func (bc *BlockChain) Reset() error {
	return bc.ResetWithGenesisBlock(bc.genesisBlock)
}

func (bc *BlockChain) ResetWithGenesisBlock(genesis *types.Block) error {

	if err := bc.SetHead(0); err != nil {
		return err
	}
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	if err := bc.hc.WriteTd(genesis.Hash(), genesis.NumberU64(), genesis.Difficulty()); err != nil {
		bc.logger.Crit("Failed to write genesis block TD", "err", err)
	}
	rawdb.WriteBlock(bc.db, genesis)

	bc.genesisBlock = genesis
	bc.insert(bc.genesisBlock)
	bc.currentBlock.Store(bc.genesisBlock)
	bc.hc.SetGenesis(bc.genesisBlock.Header())
	bc.hc.SetCurrentHeader(bc.genesisBlock.Header())
	bc.currentFastBlock.Store(bc.genesisBlock)

	return nil
}

func (bc *BlockChain) repair(head **types.Block) error {
	for {

		if _, err := state.New((*head).Root(), bc.stateCache); err == nil {
			bc.logger.Info("Rewound blockchain to past state", "number", (*head).Number(), "hash", (*head).Hash())
			return nil
		}

		block := bc.GetBlock((*head).ParentHash(), (*head).NumberU64()-1)
		if block == nil {
			return fmt.Errorf("missing block %d [%x]", (*head).NumberU64()-1, (*head).ParentHash())
		}
		*head = block
	}
}

func (bc *BlockChain) Export(w io.Writer) error {
	return bc.ExportN(w, uint64(0), bc.CurrentBlock().NumberU64())
}

func (bc *BlockChain) ExportN(w io.Writer, first uint64, last uint64) error {
	bc.chainmu.RLock()
	defer bc.chainmu.RUnlock()

	if first > last {
		return fmt.Errorf("export failed: first (%d) is greater than last (%d)", first, last)
	}
	bc.logger.Info("Exporting batch of blocks", "count", last-first+1)

	for nr := first; nr <= last; nr++ {
		block := bc.GetBlockByNumber(nr)
		if block == nil {
			return fmt.Errorf("export failed on #%d: not found", nr)
		}

		if err := block.EncodeRLP(w); err != nil {
			return err
		}
	}

	return nil
}

func (bc *BlockChain) insert(block *types.Block) {

	updateHeads := rawdb.ReadCanonicalHash(bc.db, block.NumberU64()) != block.Hash()

	rawdb.WriteCanonicalHash(bc.db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(bc.db, block.Hash())

	bc.currentBlock.Store(block)

	if updateHeads {
		bc.hc.SetCurrentHeader(block.Header())
		rawdb.WriteHeadFastBlockHash(bc.db, block.Hash())

		bc.currentFastBlock.Store(block)
	}

	ibCbMap := GetInsertBlockCbMap()
	for _, cb := range ibCbMap {
		cb(bc, block)
	}
}

func (bc *BlockChain) Genesis() *types.Block {
	return bc.genesisBlock
}

func (bc *BlockChain) GetBody(hash common.Hash) *types.Body {

	if cached, ok := bc.bodyCache.Get(hash); ok {
		body := cached.(*types.Body)
		return body
	}
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	body := rawdb.ReadBody(bc.db, hash, *number)
	if body == nil {
		return nil
	}

	bc.bodyCache.Add(hash, body)
	return body
}

func (bc *BlockChain) GetBodyRLP(hash common.Hash) rlp.RawValue {

	if cached, ok := bc.bodyRLPCache.Get(hash); ok {
		return cached.(rlp.RawValue)
	}
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	body := rawdb.ReadBodyRLP(bc.db, hash, *number)
	if len(body) == 0 {
		return nil
	}

	bc.bodyRLPCache.Add(hash, body)
	return body
}

func (bc *BlockChain) HasBlock(hash common.Hash, number uint64) bool {
	if bc.blockCache.Contains(hash) {
		return true
	}
	return rawdb.HasBody(bc.db, hash, number)
}

func (bc *BlockChain) HasState(hash common.Hash) bool {
	_, err := bc.stateCache.OpenTrie(hash)
	return err == nil
}

func (bc *BlockChain) HasBlockAndState(hash common.Hash, number uint64) bool {

	block := bc.GetBlock(hash, number)
	if block == nil {
		return false
	}
	return bc.HasState(block.Root())
}

func (bc *BlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {

	if block, ok := bc.blockCache.Get(hash); ok {
		return block.(*types.Block)
	}
	block := rawdb.ReadBlock(bc.db, hash, number)
	if block == nil {
		return nil
	}

	bc.blockCache.Add(block.Hash(), block)
	return block
}

func (bc *BlockChain) GetBlockByHash(hash common.Hash) *types.Block {
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	return bc.GetBlock(hash, *number)
}

func (bc *BlockChain) GetBlockByNumber(number uint64) *types.Block {
	hash := rawdb.ReadCanonicalHash(bc.db, number)
	if hash == (common.Hash{}) {
		return nil
	}
	return bc.GetBlock(hash, number)
}

func (bc *BlockChain) GetReceiptsByHash(hash common.Hash) types.Receipts {
	if receipts, ok := bc.receiptsCache.Get(hash); ok {
		return receipts.(types.Receipts)
	}
	number := rawdb.ReadHeaderNumber(bc.db, hash)
	if number == nil {
		return nil
	}
	receipts := rawdb.ReadReceipts(bc.db, hash, *number)
	if receipts == nil {
		return nil
	}
	bc.receiptsCache.Add(hash, receipts)
	return receipts
}

func (bc *BlockChain) GetBlocksFromHash(hash common.Hash, n int) (blocks []*types.Block) {
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	for i := 0; i < n; i++ {
		block := bc.GetBlock(hash, *number)
		if block == nil {
			break
		}
		blocks = append(blocks, block)
		hash = block.ParentHash()
		*number--
	}
	return
}

func (bc *BlockChain) GetUnclesInChain(block *types.Block, length int) []*types.Header {
	uncles := []*types.Header{}
	for i := 0; block != nil && i < length; i++ {
		uncles = append(uncles, block.Uncles()...)
		block = bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
	}
	return uncles
}

func (bc *BlockChain) ValidateBlock(block *types.Block) (*state.StateDB, types.Receipts, *types.PendingOps, error) {

	if BadHashes[block.Hash()] {
		return nil, nil, nil, ErrBlacklistedHash
	}

	if err := bc.engine.(consensus.NeatCon).VerifyHeaderBeforeConsensus(bc, block.Header(), true); err != nil {
		return nil, nil, nil, err
	}

	if err := bc.Validator().ValidateBody(block); err != nil {
		log.Debugf("ValidateBlock-ValidateBody return with error: %v", err)
		return nil, nil, nil, err
	}

	var parent *types.Block
	parent = bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
	state, err := state.New(parent.Root(), bc.stateCache)
	if err != nil {
		log.Debugf("ValidateBlock-state.New return with error: %v", err)
		return nil, nil, nil, err
	}

	receipts, _, usedGas, ops, err := bc.processor.Process(block, state, bc.vmConfig)
	if err != nil {
		log.Debugf("ValidateBlock-Process return with error: %v", err)
		return nil, nil, nil, err
	}

	err = bc.Validator().ValidateState(block, state, receipts, usedGas)
	if err != nil {
		log.Debugf("ValidateBlock-ValidateState return with error: %v", err)
		return nil, nil, nil, err
	}

	return state, receipts, ops, nil
}

func (bc *BlockChain) TrieNode(hash common.Hash) ([]byte, error) {
	return bc.stateCache.TrieDB().Node(hash)
}

func (bc *BlockChain) Stop() {
	if !atomic.CompareAndSwapInt32(&bc.running, 0, 1) {
		return
	}

	bc.scope.Close()
	close(bc.quit)
	atomic.StoreInt32(&bc.procInterrupt, 1)

	bc.wg.Wait()

	if !bc.cacheConfig.TrieDirtyDisabled {
		triedb := bc.stateCache.TrieDB()

		for _, offset := range []uint64{0, 1, triesInMemory - 1} {
			if number := bc.CurrentBlock().NumberU64(); number > offset {
				recent := bc.GetBlockByNumber(number - offset)

				bc.logger.Info("Writing cached state to disk", "block", recent.Number(), "hash", recent.Hash(), "root", recent.Root())
				if err := triedb.Commit(recent.Root(), true); err != nil {
					bc.logger.Error("Failed to commit recent state trie", "err", err)
				}
			}
		}
		for !bc.triegc.Empty() {
			triedb.Dereference(bc.triegc.PopItem().(common.Hash))
		}
		if size, _ := triedb.Size(); size != 0 {
			bc.logger.Error("Dangling trie nodes after full cleanup")
		}
	}
	bc.logger.Info("Blockchain manager stopped")
}

func (bc *BlockChain) procFutureBlocks() {
	blocks := make([]*types.Block, 0, bc.futureBlocks.Len())
	for _, hash := range bc.futureBlocks.Keys() {
		if block, exist := bc.futureBlocks.Peek(hash); exist {
			blocks = append(blocks, block.(*types.Block))
		}
	}
	if len(blocks) > 0 {
		types.BlockBy(types.Number).Sort(blocks)

		for i := range blocks {
			bc.InsertChain(blocks[i : i+1])
		}
	}
}

type WriteStatus byte

const (
	NonStatTy WriteStatus = iota
	CanonStatTy
	SideStatTy
)

func (bc *BlockChain) Rollback(chain []common.Hash) {
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	for i := len(chain) - 1; i >= 0; i-- {
		hash := chain[i]

		currentHeader := bc.hc.CurrentHeader()
		if currentHeader.Hash() == hash {
			bc.hc.SetCurrentHeader(bc.GetHeader(currentHeader.ParentHash, currentHeader.Number.Uint64()-1))
		}
		if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock.Hash() == hash {
			newFastBlock := bc.GetBlock(currentFastBlock.ParentHash(), currentFastBlock.NumberU64()-1)
			bc.currentFastBlock.Store(newFastBlock)
			rawdb.WriteHeadFastBlockHash(bc.db, newFastBlock.Hash())
		}
		if currentBlock := bc.CurrentBlock(); currentBlock.Hash() == hash {
			newBlock := bc.GetBlock(currentBlock.ParentHash(), currentBlock.NumberU64()-1)
			bc.currentBlock.Store(newBlock)
			rawdb.WriteHeadBlockHash(bc.db, newBlock.Hash())
		}
	}
}

func SetReceiptsData(config *params.ChainConfig, block *types.Block, receipts types.Receipts) error {
	signer := types.MakeSigner(config, block.Number())

	transactions, logIndex := block.Transactions(), uint(0)
	if len(transactions) != len(receipts) {
		return errors.New("transaction and receipt count mismatch")
	}

	for j := 0; j < len(receipts); j++ {

		receipts[j].TxHash = transactions[j].Hash()

		receipts[j].BlockHash = block.Hash()
		receipts[j].BlockNumber = block.Number()
		receipts[j].TransactionIndex = uint(j)

		if transactions[j].To() == nil {

			from, _ := types.Sender(signer, transactions[j])
			receipts[j].ContractAddress = crypto.CreateAddress(from, transactions[j].Nonce())
		}

		if j == 0 {
			receipts[j].GasUsed = receipts[j].CumulativeGasUsed
		} else {
			receipts[j].GasUsed = receipts[j].CumulativeGasUsed - receipts[j-1].CumulativeGasUsed
		}

		for k := 0; k < len(receipts[j].Logs); k++ {
			receipts[j].Logs[k].BlockNumber = block.NumberU64()
			receipts[j].Logs[k].BlockHash = block.Hash()
			receipts[j].Logs[k].TxHash = receipts[j].TxHash
			receipts[j].Logs[k].TxIndex = uint(j)
			receipts[j].Logs[k].Index = logIndex
			logIndex++
		}
	}
	return nil
}

func (bc *BlockChain) InsertReceiptChain(blockChain types.Blocks, receiptChain []types.Receipts) (int, error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	for i := 1; i < len(blockChain); i++ {
		if blockChain[i].NumberU64() != blockChain[i-1].NumberU64()+1 || blockChain[i].ParentHash() != blockChain[i-1].Hash() {
			bc.logger.Error("Non contiguous receipt insert", "number", blockChain[i].Number(), "hash", blockChain[i].Hash(), "parent", blockChain[i].ParentHash(),
				"prevnumber", blockChain[i-1].Number(), "prevhash", blockChain[i-1].Hash())
			return 0, fmt.Errorf("non contiguous insert: item %d is #%d [%x…], item %d is #%d [%x…] (parent [%x…])", i-1, blockChain[i-1].NumberU64(),
				blockChain[i-1].Hash().Bytes()[:4], i, blockChain[i].NumberU64(), blockChain[i].Hash().Bytes()[:4], blockChain[i].ParentHash().Bytes()[:4])
		}
	}

	var (
		stats = struct{ processed, ignored int32 }{}
		start = time.Now()
		bytes = 0
		batch = bc.db.NewBatch()
	)
	for i, block := range blockChain {
		receipts := receiptChain[i]

		if atomic.LoadInt32(&bc.procInterrupt) == 1 {
			return 0, nil
		}

		if !bc.HasHeader(block.Hash(), block.NumberU64()) {
			return i, fmt.Errorf("containing header #%d [%x…] unknown", block.Number(), block.Hash().Bytes()[:4])
		}

		if bc.HasBlock(block.Hash(), block.NumberU64()) {
			stats.ignored++
			continue
		}

		if err := SetReceiptsData(bc.chainConfig, block, receipts); err != nil {
			return i, fmt.Errorf("failed to set receipts data: %v", err)
		}

		rawdb.WriteBody(batch, block.Hash(), block.NumberU64(), block.Body())
		rawdb.WriteReceipts(batch, block.Hash(), block.NumberU64(), receipts)
		rawdb.WriteTxLookupEntries(batch, block)

		stats.processed++

		if batch.ValueSize() >= neatdb.IdealBatchSize {
			if err := batch.Write(); err != nil {
				return 0, err
			}
			bytes += batch.ValueSize()
			batch.Reset()
		}
	}
	if batch.ValueSize() > 0 {
		bytes += batch.ValueSize()
		if err := batch.Write(); err != nil {
			return 0, err
		}
	}

	bc.chainmu.Lock()
	head := blockChain[len(blockChain)-1]
	if td := bc.GetTd(head.Hash(), head.NumberU64()); td != nil {
		currentFastBlock := bc.CurrentFastBlock()
		if bc.GetTd(currentFastBlock.Hash(), currentFastBlock.NumberU64()).Cmp(td) < 0 {
			rawdb.WriteHeadFastBlockHash(bc.db, head.Hash())
			bc.currentFastBlock.Store(head)
		}
	}
	bc.chainmu.Unlock()

	bc.logger.Info("Imported new block receipts",
		"count", stats.processed,
		"elapsed", common.PrettyDuration(time.Since(start)),
		"number", head.Number(),
		"hash", head.Hash(),
		"size", common.StorageSize(bytes),
		"ignored", stats.ignored)
	return 0, nil
}

var lastWrite uint64

func (bc *BlockChain) WriteBlockWithoutState(block *types.Block, td *big.Int) (err error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	if err := bc.hc.WriteTd(block.Hash(), block.NumberU64(), td); err != nil {
		return err
	}
	rawdb.WriteBlock(bc.db, block)

	return nil
}

func (bc *BlockChain) MuLock() {
	bc.chainmu.Lock()
}

func (bc *BlockChain) MuUnLock() {
	bc.chainmu.Unlock()
}

func (bc *BlockChain) WriteBlockWithState(block *types.Block, receipts []*types.Receipt, state *state.StateDB) (status WriteStatus, err error) {

	return bc.writeBlockWithState(block, receipts, state)
}

func (bc *BlockChain) writeBlockWithState(block *types.Block, receipts []*types.Receipt, state *state.StateDB) (status WriteStatus, err error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	if bc.HasBlockAndState(block.Hash(), block.NumberU64()) {
		bc.insert(block)
		bc.futureBlocks.Remove(block.Hash())
		return CanonStatTy, nil
	}

	ptd := bc.GetTd(block.ParentHash(), block.NumberU64()-1)
	if ptd == nil {
		return NonStatTy, consensus.ErrUnknownAncestor
	}

	currentBlock := bc.CurrentBlock()
	localTd := bc.GetTd(currentBlock.Hash(), currentBlock.NumberU64())
	externTd := new(big.Int).Add(block.Difficulty(), ptd)

	if err := bc.hc.WriteTd(block.Hash(), block.NumberU64(), externTd); err != nil {
		return NonStatTy, err
	}
	rawdb.WriteBlock(bc.db, block)

	root, err := state.Commit(bc.chainConfig.IsEIP158(block.Number()))
	if err != nil {
		return NonStatTy, err
	}
	triedb := bc.stateCache.TrieDB()

	nc := bc.Engine().(consensus.NeatCon)
	FORCEFULSHWINDOW := uint64(5)
	curBlockNumber := block.NumberU64()
	curEpoch := nc.GetEpoch().GetEpochByBlockNumber(curBlockNumber)
	withinEpochSwitchWindow := curBlockNumber < curEpoch.StartBlock+FORCEFULSHWINDOW || curBlockNumber > curEpoch.EndBlock-FORCEFULSHWINDOW

	FLUSHBLOCKSINTERVAL := uint64(5000)
	meetFlushBlockInterval := curBlockNumber%FLUSHBLOCKSINTERVAL == 0

	if withinEpochSwitchWindow || bc.cacheConfig.TrieDirtyDisabled || meetFlushBlockInterval {
		if err := triedb.Commit(root, false); err != nil {
			return NonStatTy, err
		}
	} else {

		triedb.Reference(root, common.Hash{})
		bc.triegc.Push(root, -int64(block.NumberU64()))

		if current := block.NumberU64(); current > triesInMemory {

			var (
				nodes, imgs = triedb.Size()
				limit       = common.StorageSize(bc.cacheConfig.TrieDirtyLimit) * 1024 * 1024
			)
			if nodes > limit || imgs > 4*1024*1024 {
				triedb.Cap(limit - neatdb.IdealBatchSize)
			}

			chosen := current - triesInMemory

			if bc.gcproc > bc.cacheConfig.TrieTimeLimit {

				header := bc.GetHeaderByNumber(chosen)
				if header == nil {
					log.Warn("Reorg in progress, trie commit postponed", "number", chosen)
				} else {

					if chosen < lastWrite+triesInMemory && bc.gcproc >= 2*bc.cacheConfig.TrieTimeLimit {
						log.Info("State in memory for too long, committing", "time", bc.gcproc, "allowance", bc.cacheConfig.TrieTimeLimit, "optimum", float64(chosen-lastWrite)/triesInMemory)
					}

					triedb.Commit(header.Root, true)
					lastWrite = chosen
					bc.gcproc = 0
				}
			}

			for !bc.triegc.Empty() {
				root, number := bc.triegc.Pop()
				if uint64(-number) > chosen {
					bc.triegc.Push(root, number)
					break
				}
				triedb.Dereference(root.(common.Hash))
			}
		}
	}

	batch := bc.db.NewBatch()
	rawdb.WriteReceipts(batch, block.Hash(), block.NumberU64(), receipts)

	var reorg bool
	if _, ok := bc.engine.(consensus.NeatCon); ok {

		reorg = true
	} else {

		reorg := externTd.Cmp(localTd) > 0
		currentBlock = bc.CurrentBlock()
		if !reorg && externTd.Cmp(localTd) == 0 {

			reorg = block.NumberU64() < currentBlock.NumberU64() || (block.NumberU64() == currentBlock.NumberU64() && mrand.Float64() < 0.5)
		}
	}
	if reorg {

		if block.ParentHash() != currentBlock.Hash() {
			if err := bc.reorg(currentBlock, block); err != nil {
				return NonStatTy, err
			}
		}

		rawdb.WriteTxLookupEntries(batch, block)
		rawdb.WritePreimages(batch, state.Preimages())

		status = CanonStatTy
	} else {
		status = SideStatTy
	}
	if err := batch.Write(); err != nil {
		return NonStatTy, err
	}

	if status == CanonStatTy {
		bc.insert(block)
	}
	bc.futureBlocks.Remove(block.Hash())
	return status, nil
}

func (bc *BlockChain) addFutureBlock(block *types.Block) error {
	max := uint64(time.Now().Unix() + maxTimeFutureBlocks)
	if block.Time() > max {
		return fmt.Errorf("future block timestamp %v > allowed %v", block.Time(), max)
	}
	bc.futureBlocks.Add(block.Hash(), block)
	return nil
}

func (bc *BlockChain) InsertChain(chain types.Blocks) (int, error) {

	if len(chain) == 0 {
		return 0, nil
	}

	var (
		block, prev *types.Block
	)

	for i := 1; i < len(chain); i++ {
		block = chain[i]
		prev = chain[i-1]
		if block.NumberU64() != prev.NumberU64()+1 || block.ParentHash() != prev.Hash() {

			log.Error("Non contiguous block insert", "number", block.Number(), "hash", block.Hash(),
				"parent", block.ParentHash(), "prevnumber", prev.Number(), "prevhash", prev.Hash())

			return 0, fmt.Errorf("non contiguous insert: item %d is #%d [%x…], item %d is #%d [%x…] (parent [%x…])", i-1, prev.NumberU64(),
				prev.Hash().Bytes()[:4], i, block.NumberU64(), block.Hash().Bytes()[:4], block.ParentHash().Bytes()[:4])
		}
	}

	bc.wg.Add(1)
	bc.chainmu.Lock()
	n, events, logs, err := bc.insertChain(chain, true)
	bc.chainmu.Unlock()
	bc.wg.Done()

	bc.PostChainEvents(events, logs)
	return n, err
}

func (bc *BlockChain) insertChain(chain types.Blocks, verifySeals bool) (int, []interface{}, []*types.Log, error) {

	if atomic.LoadInt32(&bc.procInterrupt) == 1 {
		return 0, nil, nil, nil
	}

	var (
		stats         = insertStats{startTime: mclock.Now()}
		events        = make([]interface{}, 0, len(chain))
		lastCanon     *types.Block
		coalescedLogs []*types.Log
	)

	headers := make([]*types.Header, len(chain))
	seals := make([]bool, len(chain))

	for i, block := range chain {
		headers[i] = block.Header()
		seals[i] = verifySeals
	}

	abort, results := bc.engine.VerifyHeaders(bc, headers, seals)
	defer close(abort)

	it := newInsertIterator(chain, results, bc.validator)

	block, err := it.next()

	if err == ErrKnownBlock {

		current := bc.CurrentBlock().NumberU64()
		for block != nil && err == ErrKnownBlock {
			if current >= block.NumberU64() {
				stats.ignored++
				block, err = it.next()
			} else {
				log.Infof("this block has been written, but head not refreshed. hash %x, number %v\n",
					block.Hash(), block.NumberU64())

				err = nil
				break
			}
		}

	}

	switch {

	case err == consensus.ErrPrunedAncestor:
		return bc.insertSidechain(block, it)

	case err == consensus.ErrFutureBlock || (err == consensus.ErrUnknownAncestor && bc.futureBlocks.Contains(it.first().ParentHash())):
		for block != nil && (it.index == 0 || err == consensus.ErrUnknownAncestor) {
			if err := bc.addFutureBlock(block); err != nil {
				return it.index, events, coalescedLogs, err
			}
			block, err = it.next()
		}
		stats.queued += it.processed()
		stats.ignored += it.remaining()

		return it.index, events, coalescedLogs, err

	case err != nil:
		stats.ignored += len(it.chain)
		bc.reportBlock(block, nil, err)
		return it.index, events, coalescedLogs, err
	}

	for ; block != nil && err == nil; block, err = it.next() {

		if atomic.LoadInt32(&bc.procInterrupt) == 1 {
			bc.logger.Debug("Premature abort during blocks processing")
			break
		}

		if BadHashes[block.Hash()] {
			bc.reportBlock(block, nil, ErrBlacklistedHash)
			return it.index, events, coalescedLogs, ErrBlacklistedHash
		}

		start := time.Now()

		parent := it.previous()
		if parent == nil {
			parent = bc.GetHeader(block.ParentHash(), block.NumberU64()-1)
		}
		statedb, err := state.New(parent.Root, bc.stateCache)
		if err != nil {
			return it.index, events, coalescedLogs, err
		}

		receipts, logs, usedGas, ops, err := bc.processor.Process(block, statedb, bc.vmConfig)
		if err != nil {
			bc.reportBlock(block, receipts, err)
			return it.index, events, coalescedLogs, err
		}

		err = bc.Validator().ValidateState(block, statedb, receipts, usedGas)
		if err != nil {
			bc.reportBlock(block, receipts, err)
			return it.index, events, coalescedLogs, err
		}
		proctime := time.Since(start)

		status, err := bc.writeBlockWithState(block, receipts, statedb)
		if err != nil {
			return it.index, events, coalescedLogs, err
		}

		for _, op := range ops.Ops() {
			if err := ApplyOp(op, bc, bc.cch); err != nil {
				bc.logger.Error("Failed executing op", op, "err", err)
			}
		}

		blockInsertTimer.UpdateSince(start)

		switch status {
		case CanonStatTy:
			bc.logger.Debug("Inserted new block", "number", block.Number(), "hash", block.Hash(),
				"uncles", len(block.Uncles()), "txs", len(block.Transactions()), "gas", block.GasUsed(),
				"elapsed", common.PrettyDuration(time.Since(start)),
				"root", block.Root())

			coalescedLogs = append(coalescedLogs, logs...)
			events = append(events, ChainEvent{block, block.Hash(), logs})
			lastCanon = block

			bc.gcproc += proctime

		case SideStatTy:
			bc.logger.Debug("Inserted forked block", "number", block.Number(), "hash", block.Hash(),
				"diff", block.Difficulty(), "elapsed", common.PrettyDuration(time.Since(start)),
				"txs", len(block.Transactions()), "gas", block.GasUsed(), "uncles", len(block.Uncles()),
				"root", block.Root())
			events = append(events, ChainSideEvent{block})
		}
		stats.processed++
		stats.usedGas += usedGas

		dirty, _ := bc.stateCache.TrieDB().Size()
		stats.report(chain, it.index, dirty)
	}

	if block != nil && err == consensus.ErrFutureBlock {
		if err := bc.addFutureBlock(block); err != nil {
			return it.index, events, coalescedLogs, err
		}
		block, err = it.next()

		for ; block != nil && err == consensus.ErrUnknownAncestor; block, err = it.next() {
			if err := bc.addFutureBlock(block); err != nil {
				return it.index, events, coalescedLogs, err
			}
			stats.queued++
		}
	}
	stats.ignored += it.remaining()

	if lastCanon != nil && bc.CurrentBlock().Hash() == lastCanon.Hash() {
		events = append(events, ChainHeadEvent{lastCanon})
	}

	return it.index, events, coalescedLogs, err
}

func (bc *BlockChain) insertSidechain(block *types.Block, it *insertIterator) (int, []interface{}, []*types.Log, error) {
	var (
		externTd *big.Int
		current  = bc.CurrentBlock()
	)

	err := consensus.ErrPrunedAncestor
	for ; block != nil && (err == consensus.ErrPrunedAncestor); block, err = it.next() {

		if number := block.NumberU64(); current.NumberU64() >= number {
			canonical := bc.GetBlockByNumber(number)
			if canonical != nil && canonical.Hash() == block.Hash() {

				continue
			}
			if canonical != nil && canonical.Root() == block.Root() {

				bc.logger.Warn("Sidechain ghost-state attack detected", "number", block.NumberU64(), "sideroot", block.Root(), "canonroot", canonical.Root())

				return it.index, nil, nil, errors.New("sidechain ghost-state attack")
			}
		}
		if externTd == nil {
			externTd = bc.GetTd(block.ParentHash(), block.NumberU64()-1)
		}
		externTd = new(big.Int).Add(externTd, block.Difficulty())

		if !bc.HasBlock(block.Hash(), block.NumberU64()) {
			start := time.Now()
			if err := bc.WriteBlockWithoutState(block, externTd); err != nil {
				return it.index, nil, nil, err
			}
			bc.logger.Debug("Injected sidechain block", "number", block.Number(), "hash", block.Hash(),
				"diff", block.Difficulty(), "elapsed", common.PrettyDuration(time.Since(start)),
				"txs", len(block.Transactions()), "gas", block.GasUsed(), "uncles", len(block.Uncles()),
				"root", block.Root())
		}
	}

	localTd := bc.GetTd(current.Hash(), current.NumberU64())
	if localTd.Cmp(externTd) > 0 {
		bc.logger.Info("Sidechain written to disk", "start", it.first().NumberU64(), "end", it.previous().Number, "sidetd", externTd, "localtd", localTd)
		return it.index, nil, nil, err
	}

	var (
		hashes  []common.Hash
		numbers []uint64
	)
	parent := it.previous()
	for parent != nil && !bc.HasState(parent.Root) {
		hashes = append(hashes, parent.Hash())
		numbers = append(numbers, parent.Number.Uint64())

		parent = bc.GetHeader(parent.ParentHash, parent.Number.Uint64()-1)
	}
	if parent == nil {
		return it.index, nil, nil, errors.New("missing parent")
	}

	var (
		blocks []*types.Block
		memory common.StorageSize
	)
	for i := len(hashes) - 1; i >= 0; i-- {

		block := bc.GetBlock(hashes[i], numbers[i])

		blocks = append(blocks, block)
		memory += block.Size()

		if len(blocks) >= 2048 || memory > 64*1024*1024 {
			bc.logger.Info("Importing heavy sidechain segment", "blocks", len(blocks), "start", blocks[0].NumberU64(), "end", block.NumberU64())
			if _, _, _, err := bc.insertChain(blocks, false); err != nil {
				return 0, nil, nil, err
			}
			blocks, memory = blocks[:0], 0

			if atomic.LoadInt32(&bc.procInterrupt) == 1 {
				bc.logger.Debug("Premature abort during blocks processing")
				return 0, nil, nil, nil
			}
		}
	}
	if len(blocks) > 0 {
		bc.logger.Info("Importing sidechain segment", "start", blocks[0].NumberU64(), "end", blocks[len(blocks)-1].NumberU64())
		return bc.insertChain(blocks, false)
	}
	return 0, nil, nil, nil
}

func (bc *BlockChain) reorg(oldBlock, newBlock *types.Block) error {
	var (
		newChain    types.Blocks
		oldChain    types.Blocks
		commonBlock *types.Block

		deletedTxs types.Transactions

		deletedLogs []*types.Log

		collectLogs = func(hash common.Hash) {
			number := bc.hc.GetBlockNumber(hash)
			if number == nil {
				return
			}

			receipts := rawdb.ReadReceipts(bc.db, hash, *number)
			for _, receipt := range receipts {
				for _, log := range receipt.Logs {
					del := *log
					del.Removed = true
					deletedLogs = append(deletedLogs, &del)
				}
			}
		}
	)

	if oldBlock.NumberU64() > newBlock.NumberU64() {

		for ; oldBlock != nil && oldBlock.NumberU64() != newBlock.NumberU64(); oldBlock = bc.GetBlock(oldBlock.ParentHash(), oldBlock.NumberU64()-1) {
			oldChain = append(oldChain, oldBlock)
			deletedTxs = append(deletedTxs, oldBlock.Transactions()...)
			collectLogs(oldBlock.Hash())
		}
	} else {

		for ; newBlock != nil && newBlock.NumberU64() != oldBlock.NumberU64(); newBlock = bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1) {
			newChain = append(newChain, newBlock)
		}
	}
	if oldBlock == nil {
		return fmt.Errorf("Invalid old chain")
	}
	if newBlock == nil {
		return fmt.Errorf("Invalid new chain")
	}

	for {
		if oldBlock.Hash() == newBlock.Hash() {
			commonBlock = oldBlock
			break
		}

		oldChain = append(oldChain, oldBlock)
		newChain = append(newChain, newBlock)
		deletedTxs = append(deletedTxs, oldBlock.Transactions()...)
		collectLogs(oldBlock.Hash())

		oldBlock, newBlock = bc.GetBlock(oldBlock.ParentHash(), oldBlock.NumberU64()-1), bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1)
		if oldBlock == nil {
			return fmt.Errorf("Invalid old chain")
		}
		if newBlock == nil {
			return fmt.Errorf("Invalid new chain")
		}
	}

	if len(oldChain) > 0 && len(newChain) > 0 {
		logFn := log.Debug
		if len(oldChain) > 63 {
			logFn = log.Warn
		}
		logFn("Chain split detected", "number", commonBlock.Number(), "hash", commonBlock.Hash(),
			"drop", len(oldChain), "dropfrom", oldChain[0].Hash(), "add", len(newChain), "addfrom", newChain[0].Hash())
	} else {
		bc.logger.Error("Impossible reorg, please file an issue", "oldnum", oldBlock.Number(), "oldhash", oldBlock.Hash(), "newnum", newBlock.Number(), "newhash", newBlock.Hash())
	}

	var addedTxs types.Transactions
	for i := len(newChain) - 1; i >= 0; i-- {

		bc.insert(newChain[i])

		rawdb.WriteTxLookupEntries(bc.db, newChain[i])
		addedTxs = append(addedTxs, newChain[i].Transactions()...)
	}

	batch := bc.db.NewBatch()
	for _, tx := range types.TxDifference(deletedTxs, addedTxs) {
		rawdb.DeleteTxLookupEntry(batch, tx.Hash())
	}
	batch.Write()

	if len(deletedLogs) > 0 {
		go bc.rmLogsFeed.Send(RemovedLogsEvent{deletedLogs})
	}
	if len(oldChain) > 0 {
		go func() {
			for _, block := range oldChain {
				bc.chainSideFeed.Send(ChainSideEvent{Block: block})
			}
		}()
	}

	return nil
}

func (bc *BlockChain) PostChainEvents(events []interface{}, logs []*types.Log) {

	if logs != nil {
		bc.logsFeed.Send(logs)
	}
	for _, event := range events {
		switch ev := event.(type) {
		case ChainEvent:
			bc.chainFeed.Send(ev)

		case ChainHeadEvent:
			bc.chainHeadFeed.Send(ev)

		case ChainSideEvent:
			bc.chainSideFeed.Send(ev)

		case CreateSideChainEvent:
			bc.createSideChainFeed.Send(ev)

		case StartMiningEvent:
			bc.startMiningFeed.Send(ev)

		case StopMiningEvent:
			bc.stopMiningFeed.Send(ev)
		}
	}
}

func (bc *BlockChain) update() {
	futureTimer := time.NewTicker(5 * time.Second)
	defer futureTimer.Stop()
	for {
		select {
		case <-futureTimer.C:
			bc.procFutureBlocks()
		case <-bc.quit:
			return
		}
	}
}

type BadBlockArgs struct {
	Hash   common.Hash   `json:"hash"`
	Header *types.Header `json:"header"`
}

func (bc *BlockChain) BadBlocks() ([]BadBlockArgs, error) {
	headers := make([]BadBlockArgs, 0, bc.badBlocks.Len())
	for _, hash := range bc.badBlocks.Keys() {
		if hdr, exist := bc.badBlocks.Peek(hash); exist {
			header := hdr.(*types.Header)
			headers = append(headers, BadBlockArgs{header.Hash(), header})
		}
	}
	return headers, nil
}

func (bc *BlockChain) HasBadBlock(hash common.Hash) bool {
	return bc.badBlocks.Contains(hash)
}

func (bc *BlockChain) addBadBlock(block *types.Block) {
	bc.badBlocks.Add(block.Header().Hash(), block.Header())
}

func (bc *BlockChain) reportBlock(block *types.Block, receipts types.Receipts, err error) {
	bc.addBadBlock(block)

	var receiptString string
	for _, receipt := range receipts {
		receiptString += fmt.Sprintf("\t%v\n", receipt)
	}
	bc.logger.Error(fmt.Sprintf(`
########## BAD BLOCK #########
Chain config: %v

Number: %v
Hash: 0x%x
%v

Error: %v
##############################
`, bc.chainConfig, block.Number(), block.Hash(), receiptString, err))
}

func (bc *BlockChain) InsertHeaderChain(chain []*types.Header, checkFreq int) (int, error) {
	start := time.Now()
	if i, err := bc.hc.ValidateHeaderChain(chain, checkFreq); err != nil {
		return i, err
	}

	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	bc.wg.Add(1)
	defer bc.wg.Done()

	whFunc := func(header *types.Header) error {
		_, err := bc.hc.WriteHeader(header)
		return err
	}

	return bc.hc.InsertHeaderChain(chain, whFunc, start)
}

func (bc *BlockChain) CurrentHeader() *types.Header {
	return bc.hc.CurrentHeader()
}

func (bc *BlockChain) GetTd(hash common.Hash, number uint64) *big.Int {
	return bc.hc.GetTd(hash, number)
}

func (bc *BlockChain) GetTdByHash(hash common.Hash) *big.Int {
	return bc.hc.GetTdByHash(hash)
}

func (bc *BlockChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	return bc.hc.GetHeader(hash, number)
}

func (bc *BlockChain) GetHeaderByHash(hash common.Hash) *types.Header {
	return bc.hc.GetHeaderByHash(hash)
}

func (bc *BlockChain) HasHeader(hash common.Hash, number uint64) bool {
	return bc.hc.HasHeader(hash, number)
}

func (bc *BlockChain) GetBlockHashesFromHash(hash common.Hash, max uint64) []common.Hash {
	return bc.hc.GetBlockHashesFromHash(hash, max)
}

func (bc *BlockChain) GetHeaderByNumber(number uint64) *types.Header {
	return bc.hc.GetHeaderByNumber(number)
}

func (bc *BlockChain) Config() *params.ChainConfig { return bc.chainConfig }

func (bc *BlockChain) Engine() consensus.Engine { return bc.engine }

func (bc *BlockChain) GetCrossChainHelper() CrossChainHelper { return bc.cch }

func (bc *BlockChain) SubscribeRemovedLogsEvent(ch chan<- RemovedLogsEvent) event.Subscription {
	return bc.scope.Track(bc.rmLogsFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeChainEvent(ch chan<- ChainEvent) event.Subscription {
	return bc.scope.Track(bc.chainFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription {
	return bc.scope.Track(bc.chainHeadFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeChainSideEvent(ch chan<- ChainSideEvent) event.Subscription {
	return bc.scope.Track(bc.chainSideFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return bc.scope.Track(bc.logsFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeCreateSideChainEvent(ch chan<- CreateSideChainEvent) event.Subscription {
	return bc.scope.Track(bc.createSideChainFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeStartMiningEvent(ch chan<- StartMiningEvent) event.Subscription {
	return bc.scope.Track(bc.startMiningFeed.Subscribe(ch))
}

func (bc *BlockChain) SubscribeStopMiningEvent(ch chan<- StopMiningEvent) event.Subscription {
	return bc.scope.Track(bc.stopMiningFeed.Subscribe(ch))
}
