package neatptc

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/nio-net/nio/chain/core"
	"github.com/nio-net/nio/chain/core/datareduction"
	"github.com/nio-net/nio/chain/core/rawdb"
	"github.com/nio-net/nio/chain/core/state"
	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/chain/trie"
	"github.com/nio-net/nio/network/rpc"
	"github.com/nio-net/nio/params"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/common/hexutil"
	"github.com/nio-net/nio/utilities/crypto"
	"github.com/nio-net/nio/utilities/miner"
	"github.com/nio-net/nio/utilities/rlp"
)

type PublicEthereumAPI struct {
	e *NeatIO
}

func NewPublicEthereumAPI(e *NeatIO) *PublicEthereumAPI {
	return &PublicEthereumAPI{e}
}

func (api *PublicEthereumAPI) Etherbase() (string, error) {
	eb, err := api.e.Coinbase()
	return eb.String(), err
}

func (api *PublicEthereumAPI) Coinbase() (string, error) {
	return api.Etherbase()
}

type PublicMinerAPI struct {
	e     *NeatIO
	agent *miner.RemoteAgent
}

func NewPublicMinerAPI(e *NeatIO) *PublicMinerAPI {
	agent := miner.NewRemoteAgent(e.BlockChain(), e.Engine())
	if e.Miner() != nil {
		e.Miner().Register(agent)
	}

	return &PublicMinerAPI{e, agent}
}

func (api *PublicMinerAPI) Mining() bool {
	if api.e.Miner() != nil {
		return api.e.IsMining()
	}
	return false
}

func (api *PublicMinerAPI) SubmitWork(nonce types.BlockNonce, solution, digest common.Hash) bool {
	return api.agent.SubmitWork(nonce, digest, solution)
}

func (api *PublicMinerAPI) GetWork() ([3]string, error) {
	if !api.e.IsMining() {
		if err := api.e.StartMining(false); err != nil {
			return [3]string{}, err
		}
	}
	work, err := api.agent.GetWork()
	if err != nil {
		return work, fmt.Errorf("mining not ready: %v", err)
	}
	return work, nil
}

func (api *PublicMinerAPI) SubmitHashrate(hashrate hexutil.Uint64, id common.Hash) bool {
	api.agent.SubmitHashrate(id, uint64(hashrate))
	return true
}

type PrivateMinerAPI struct {
	e *NeatIO
}

func NewPrivateMinerAPI(e *NeatIO) *PrivateMinerAPI {
	return &PrivateMinerAPI{e: e}
}

func (api *PrivateMinerAPI) Start(threads *int) error {

	if threads == nil {
		threads = new(int)
	} else if *threads == 0 {
		*threads = -1
	}
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := api.e.engine.(threaded); ok {
		log.Info("Updated mining threads", "threads", *threads)
		th.SetThreads(*threads)
	}

	if !api.e.IsMining() {

		api.e.lock.RLock()
		price := api.e.gasPrice
		api.e.lock.RUnlock()

		api.e.txPool.SetGasPrice(price)
		return api.e.StartMining(true)
	}
	return nil
}

func (api *PrivateMinerAPI) Stop() bool {
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := api.e.engine.(threaded); ok {
		th.SetThreads(-1)
	}
	api.e.StopMining()
	return true
}

func (api *PrivateMinerAPI) SetExtra(extra string) (bool, error) {
	if err := api.e.Miner().SetExtra([]byte(extra)); err != nil {
		return false, err
	}
	return true, nil
}

func (api *PrivateMinerAPI) SetGasPrice(gasPrice hexutil.Big) bool {
	api.e.lock.Lock()
	api.e.gasPrice = (*big.Int)(&gasPrice)
	api.e.lock.Unlock()

	api.e.txPool.SetGasPrice((*big.Int)(&gasPrice))
	return true
}

func (api *PrivateMinerAPI) SetCoinbase(coinbase common.Address) bool {
	api.e.SetCoinbase(coinbase)
	return true
}

type PrivateAdminAPI struct {
	eth *NeatIO
}

func NewPrivateAdminAPI(eth *NeatIO) *PrivateAdminAPI {
	return &PrivateAdminAPI{eth: eth}
}

func (api *PrivateAdminAPI) ExportChain(file string) (bool, error) {

	out, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return false, err
	}
	defer out.Close()

	var writer io.Writer = out
	if strings.HasSuffix(file, ".gz") {
		writer = gzip.NewWriter(writer)
		defer writer.(*gzip.Writer).Close()
	}

	if err := api.eth.BlockChain().Export(writer); err != nil {
		return false, err
	}
	return true, nil
}

func hasAllBlocks(chain *core.BlockChain, bs []*types.Block) bool {
	for _, b := range bs {
		if !chain.HasBlock(b.Hash(), b.NumberU64()) {
			return false
		}
	}

	return true
}

func (api *PrivateAdminAPI) ImportChain(file string) (bool, error) {

	in, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(file, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return false, err
		}
	}

	stream := rlp.NewStream(reader, 0)

	blocks, index := make([]*types.Block, 0, 2500), 0
	for batch := 0; ; batch++ {

		for len(blocks) < cap(blocks) {
			block := new(types.Block)
			if err := stream.Decode(block); err == io.EOF {
				break
			} else if err != nil {
				return false, fmt.Errorf("block %d: failed to parse: %v", index, err)
			}
			blocks = append(blocks, block)
			index++
		}
		if len(blocks) == 0 {
			break
		}

		if hasAllBlocks(api.eth.BlockChain(), blocks) {
			blocks = blocks[:0]
			continue
		}

		if _, err := api.eth.BlockChain().InsertChain(blocks); err != nil {
			return false, fmt.Errorf("batch %d: failed to insert: %v", batch, err)
		}
		blocks = blocks[:0]
	}
	return true, nil
}

func (api *PrivateAdminAPI) PruneStateData(height *hexutil.Uint64) (bool, error) {
	var blockNumber uint64
	if height != nil && *height > 0 {
		blockNumber = uint64(*height)
	}

	go api.eth.StartScanAndPrune(blockNumber)
	return true, nil
}

func (api *PrivateAdminAPI) LatestPruneState() (*datareduction.PruneStatus, error) {
	status := datareduction.GetLatestStatus(api.eth.pruneDb)
	status.LatestBlockNumber = api.eth.blockchain.CurrentHeader().Number.Uint64()
	return status, nil
}

type PublicDebugAPI struct {
	eth *NeatIO
}

func NewPublicDebugAPI(eth *NeatIO) *PublicDebugAPI {
	return &PublicDebugAPI{eth: eth}
}

func (api *PublicDebugAPI) DumpBlock(blockNr rpc.BlockNumber) (state.Dump, error) {
	if blockNr == rpc.PendingBlockNumber {

		_, stateDb := api.eth.miner.Pending()
		return stateDb.RawDump(), nil
	}
	var block *types.Block
	if blockNr == rpc.LatestBlockNumber {
		block = api.eth.blockchain.CurrentBlock()
	} else {
		block = api.eth.blockchain.GetBlockByNumber(uint64(blockNr))
	}
	if block == nil {
		return state.Dump{}, fmt.Errorf("block #%d not found", blockNr)
	}
	stateDb, err := api.eth.BlockChain().StateAt(block.Root())
	if err != nil {
		return state.Dump{}, err
	}
	return stateDb.RawDump(), nil
}

type PrivateDebugAPI struct {
	config *params.ChainConfig
	eth    *NeatIO
}

func NewPrivateDebugAPI(config *params.ChainConfig, eth *NeatIO) *PrivateDebugAPI {
	return &PrivateDebugAPI{config: config, eth: eth}
}

func (api *PrivateDebugAPI) Preimage(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	if preimage := rawdb.ReadPreimage(api.eth.ChainDb(), hash); preimage != nil {
		return preimage, nil
	}
	return nil, errors.New("unknown preimage")
}

func (api *PrivateDebugAPI) RemotePreimage(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	peer := api.eth.protocolManager.peers.BestPeer()

	hashes := make([]common.Hash, 0)
	return nil, peer.RequestPreimages(append(hashes, hash))
}

func (api *PrivateDebugAPI) RemovePreimage(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	rawdb.DeletePreimage(api.eth.ChainDb(), hash)
	return nil, nil
}

func (api *PrivateDebugAPI) BrokenPreimage(ctx context.Context, hash common.Hash, preimage hexutil.Bytes) (hexutil.Bytes, error) {

	rawdb.WritePreimages(api.eth.ChainDb(), map[common.Hash][]byte{hash: preimage})

	if read_preimage := rawdb.ReadPreimage(api.eth.ChainDb(), hash); read_preimage != nil {
		return read_preimage, nil
	}
	return nil, errors.New("broken preimage failed")
}

func (api *PrivateDebugAPI) FindBadPreimage(ctx context.Context) (interface{}, error) {

	images := make(map[common.Hash]string)

	db := api.eth.ChainDb()
	it := db.NewIteratorWithPrefix([]byte("secure-key-"))
	for it.Next() {
		keyHash := common.BytesToHash(it.Key())
		valueHash := crypto.Keccak256Hash(it.Value())
		if keyHash != valueHash {

			images[keyHash] = common.Bytes2Hex(it.Value())
		}
	}
	it.Release()

	return images, nil
}

func (api *PrivateDebugAPI) GetBadBlocks(ctx context.Context) ([]core.BadBlockArgs, error) {
	return api.eth.BlockChain().BadBlocks()
}

type StorageRangeResult struct {
	Storage storageMap   `json:"storage"`
	NextKey *common.Hash `json:"nextKey"`
}

type storageMap map[common.Hash]storageEntry

type storageEntry struct {
	Key   *common.Hash `json:"key"`
	Value common.Hash  `json:"value"`
}

func (api *PrivateDebugAPI) StorageRangeAt(ctx context.Context, blockHash common.Hash, txIndex int, contractAddress common.Address, keyStart hexutil.Bytes, maxResult int) (StorageRangeResult, error) {
	_, _, statedb, err := api.computeTxEnv(blockHash, txIndex, 0)
	if err != nil {
		return StorageRangeResult{}, err
	}
	st := statedb.StorageTrie(contractAddress)
	if st == nil {
		return StorageRangeResult{}, fmt.Errorf("account %x doesn't exist", contractAddress)
	}
	return storageRangeAt(st, keyStart, maxResult)
}

func storageRangeAt(st state.Trie, start []byte, maxResult int) (StorageRangeResult, error) {
	it := trie.NewIterator(st.NodeIterator(start))
	result := StorageRangeResult{Storage: storageMap{}}
	for i := 0; i < maxResult && it.Next(); i++ {
		_, content, _, err := rlp.Split(it.Value)
		if err != nil {
			return StorageRangeResult{}, err
		}
		e := storageEntry{Value: common.BytesToHash(content)}
		if preimage := st.GetKey(it.Key); preimage != nil {
			preimage := common.BytesToHash(preimage)
			e.Key = &preimage
		}
		result.Storage[common.BytesToHash(it.Key)] = e
	}

	if it.Next() {
		next := common.BytesToHash(it.Key)
		result.NextKey = &next
	}
	return result, nil
}

func (api *PrivateDebugAPI) GetModifiedAccountsByNumber(startNum uint64, endNum *uint64) ([]common.Address, error) {
	var startBlock, endBlock *types.Block

	startBlock = api.eth.blockchain.GetBlockByNumber(startNum)
	if startBlock == nil {
		return nil, fmt.Errorf("start block %x not found", startNum)
	}

	if endNum == nil {
		endBlock = startBlock
		startBlock = api.eth.blockchain.GetBlockByHash(startBlock.ParentHash())
		if startBlock == nil {
			return nil, fmt.Errorf("block %x has no parent", endBlock.Number())
		}
	} else {
		endBlock = api.eth.blockchain.GetBlockByNumber(*endNum)
		if endBlock == nil {
			return nil, fmt.Errorf("end block %d not found", *endNum)
		}
	}
	return api.getModifiedAccounts(startBlock, endBlock)
}

func (api *PrivateDebugAPI) GetModifiedAccountsByHash(startHash common.Hash, endHash *common.Hash) ([]common.Address, error) {
	var startBlock, endBlock *types.Block
	startBlock = api.eth.blockchain.GetBlockByHash(startHash)
	if startBlock == nil {
		return nil, fmt.Errorf("start block %x not found", startHash)
	}

	if endHash == nil {
		endBlock = startBlock
		startBlock = api.eth.blockchain.GetBlockByHash(startBlock.ParentHash())
		if startBlock == nil {
			return nil, fmt.Errorf("block %x has no parent", endBlock.Number())
		}
	} else {
		endBlock = api.eth.blockchain.GetBlockByHash(*endHash)
		if endBlock == nil {
			return nil, fmt.Errorf("end block %x not found", *endHash)
		}
	}
	return api.getModifiedAccounts(startBlock, endBlock)
}

func (api *PrivateDebugAPI) getModifiedAccounts(startBlock, endBlock *types.Block) ([]common.Address, error) {
	if startBlock.Number().Uint64() >= endBlock.Number().Uint64() {
		return nil, fmt.Errorf("start block height (%d) must be less than end block height (%d)", startBlock.Number().Uint64(), endBlock.Number().Uint64())
	}
	triedb := api.eth.BlockChain().StateCache().TrieDB()

	oldTrie, err := trie.NewSecure(startBlock.Root(), triedb)
	if err != nil {
		return nil, err
	}
	newTrie, err := trie.NewSecure(endBlock.Root(), triedb)
	if err != nil {
		return nil, err
	}

	diff, _ := trie.NewDifferenceIterator(oldTrie.NodeIterator([]byte{}), newTrie.NodeIterator([]byte{}))
	iter := trie.NewIterator(diff)

	var dirty []common.Address
	for iter.Next() {
		key := newTrie.GetKey(iter.Key)
		if key == nil {
			return nil, fmt.Errorf("no preimage found for hash %x", iter.Key)
		}
		dirty = append(dirty, common.BytesToAddress(key))
	}
	return dirty, nil
}

func (api *PrivateDebugAPI) ReadRawDBNode(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	if exist, _ := api.eth.ChainDb().Has(hash.Bytes()); exist {
		return api.eth.ChainDb().Get(hash.Bytes())
	}
	return nil, errors.New("key not exist")
}

func (api *PrivateDebugAPI) BroadcastRawDBNode(ctx context.Context, hash common.Hash) (map[string]error, error) {
	result := make(map[string]error)
	if exist, _ := api.eth.ChainDb().Has(hash.Bytes()); exist {
		data, _ := api.eth.chainDb.Get(hash.Bytes())

		for _, peer := range api.eth.protocolManager.peers.Peers() {
			result[peer.id] = peer.SendTrieNodeData([][]byte{data})
		}
	}
	return result, nil
}

type resultNode struct {
	Key common.Hash   `json:"hash"`
	Val hexutil.Bytes `json:"value"`
}

func (api *PrivateDebugAPI) PrintTrieNode(ctx context.Context, root common.Hash) ([]resultNode, error) {
	result := make([]resultNode, 0)
	t, _ := trie.NewSecure(root, trie.NewDatabase(api.eth.chainDb))
	it := t.NodeIterator(nil)
	for it.Next(true) {
		if !it.Leaf() {
			h := it.Hash()
			v, _ := api.eth.chainDb.Get(h.Bytes())
			result = append(result, resultNode{h, v})
		}
	}
	return result, it.Error()
}
