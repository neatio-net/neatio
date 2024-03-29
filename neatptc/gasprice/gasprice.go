package gasprice

import (
	"context"
	"math/big"
	"sort"
	"sync"

	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/internal/neatapi"
	"github.com/neatio-net/neatio/network/rpc"
	"github.com/neatio-net/neatio/params"
	"github.com/neatio-net/neatio/utilities/common"
)

var maxPrice = big.NewInt(5000 * params.GWei)

type Config struct {
	Blocks     int
	Percentile int
	Default    *big.Int `toml:",omitempty"`
}

type Oracle struct {
	backend   neatapi.Backend
	lastHead  common.Hash
	lastPrice *big.Int
	cacheLock sync.RWMutex
	fetchLock sync.Mutex

	checkBlocks, maxEmpty, maxBlocks int
	percentile                       int
}

func NewOracle(backend neatapi.Backend, params Config) *Oracle {
	blocks := params.Blocks
	if blocks < 1 {
		blocks = 1
	}
	percent := params.Percentile
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return &Oracle{
		backend:     backend,
		lastPrice:   params.Default,
		checkBlocks: blocks,
		maxEmpty:    blocks / 2,
		maxBlocks:   blocks * 5,
		percentile:  percent,
	}
}

func (gpo *Oracle) SuggestPrice(ctx context.Context) (*big.Int, error) {
	gpo.cacheLock.RLock()
	lastHead := gpo.lastHead
	lastPrice := gpo.lastPrice
	gpo.cacheLock.RUnlock()

	head, _ := gpo.backend.HeaderByNumber(ctx, rpc.LatestBlockNumber)
	headHash := head.Hash()
	if headHash == lastHead {
		return lastPrice, nil
	}

	gpo.fetchLock.Lock()
	defer gpo.fetchLock.Unlock()

	gpo.cacheLock.RLock()
	lastHead = gpo.lastHead
	lastPrice = gpo.lastPrice
	gpo.cacheLock.RUnlock()
	if headHash == lastHead {
		return lastPrice, nil
	}

	blockNum := head.Number.Uint64()
	ch := make(chan getBlockPricesResult, gpo.checkBlocks)
	sent := 0
	exp := 0
	var blockPrices []*big.Int
	for sent < gpo.checkBlocks && blockNum > 0 {
		go gpo.getBlockPrices(ctx, types.MakeSigner(gpo.backend.ChainConfig(), big.NewInt(int64(blockNum))), blockNum, ch)
		sent++
		exp++
		blockNum--
	}
	maxEmpty := gpo.maxEmpty
	for exp > 0 {
		res := <-ch
		if res.err != nil {
			return lastPrice, res.err
		}
		exp--
		if res.price != nil {
			blockPrices = append(blockPrices, res.price)
			continue
		}
		if maxEmpty > 0 {
			maxEmpty--
			continue
		}
		if blockNum > 0 && sent < gpo.maxBlocks {
			go gpo.getBlockPrices(ctx, types.MakeSigner(gpo.backend.ChainConfig(), big.NewInt(int64(blockNum))), blockNum, ch)
			sent++
			exp++
			blockNum--
		}
	}
	price := lastPrice
	if len(blockPrices) > 0 {
		sort.Sort(bigIntArray(blockPrices))
		price = blockPrices[(len(blockPrices)-1)*gpo.percentile/100]
	}
	if price.Cmp(maxPrice) > 0 {
		price = new(big.Int).Set(maxPrice)
	}

	gpo.cacheLock.Lock()
	gpo.lastHead = headHash
	gpo.lastPrice = price
	gpo.cacheLock.Unlock()
	return price, nil
}

type getBlockPricesResult struct {
	price *big.Int
	err   error
}

type transactionsByGasPrice []*types.Transaction

func (t transactionsByGasPrice) Len() int           { return len(t) }
func (t transactionsByGasPrice) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t transactionsByGasPrice) Less(i, j int) bool { return t[i].GasPrice().Cmp(t[j].GasPrice()) < 0 }

func (gpo *Oracle) getBlockPrices(ctx context.Context, signer types.Signer, blockNum uint64, ch chan getBlockPricesResult) {
	block, err := gpo.backend.BlockByNumber(ctx, rpc.BlockNumber(blockNum))
	if block == nil {
		ch <- getBlockPricesResult{nil, err}
		return
	}

	blockTxs := block.Transactions()
	txs := make([]*types.Transaction, len(blockTxs))
	copy(txs, blockTxs)
	sort.Sort(transactionsByGasPrice(txs))

	for _, tx := range txs {
		sender, err := types.Sender(signer, tx)
		if err == nil && sender != block.Coinbase() {
			ch <- getBlockPricesResult{tx.GasPrice(), nil}
			return
		}
	}
	ch <- getBlockPricesResult{nil, nil}
}

type bigIntArray []*big.Int

func (s bigIntArray) Len() int           { return len(s) }
func (s bigIntArray) Less(i, j int) bool { return s[i].Cmp(s[j]) < 0 }
func (s bigIntArray) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
