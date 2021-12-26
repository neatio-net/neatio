package neatapi

import (
	"context"
	"math/big"

	"github.com/neatlab/neatio/chain/accounts"
	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/chain/core/state"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/core/vm"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/neatptc/downloader"
	"github.com/neatlab/neatio/network/rpc"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/event"
)

type Backend interface {
	Downloader() *downloader.Downloader
	ProtocolVersion() int
	SuggestPrice(ctx context.Context) (*big.Int, error)
	ChainDb() neatdb.Database
	EventMux() *event.TypeMux
	AccountManager() *accounts.Manager

	SetHead(number uint64)
	HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error)
	BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error)
	StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error)
	GetBlock(ctx context.Context, blockHash common.Hash) (*types.Block, error)
	GetReceipts(ctx context.Context, blockHash common.Hash) (types.Receipts, error)
	GetTd(blockHash common.Hash) *big.Int
	GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error)
	SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription
	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription

	SendTx(ctx context.Context, signedTx *types.Transaction) error
	GetPoolTransactions() (types.Transactions, error)
	GetPoolTransaction(txHash common.Hash) *types.Transaction
	GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error)
	Stats() (pending int, queued int)
	TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions)
	SubscribeTxPreEvent(chan<- core.TxPreEvent) event.Subscription

	ChainConfig() *params.ChainConfig
	CurrentBlock() *types.Block

	GetCrossChainHelper() core.CrossChainHelper

	BroadcastTX3ProofData(proofData *types.TX3ProofData)
}

func GetAPIs(apiBackend Backend, solcPath string) []rpc.API {
	compiler := makeCompilerAPIs(solcPath)
	nonceLock := new(AddrLocker)
	txapi := NewPublicTransactionPoolAPI(apiBackend, nonceLock)

	all := []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicNEATChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicBlockChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   txapi,
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   NewPublicNEATChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   NewPublicBlockChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   txapi,
			Public:    true,
		}, {
			Namespace: "txpool",
			Version:   "1.0",
			Service:   NewPublicTxPoolAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(apiBackend),
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicAccountAPI(apiBackend.AccountManager()),
			Public:    true,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   NewPublicAccountAPI(apiBackend.AccountManager()),
			Public:    true,
		}, {
			Namespace: "personal",
			Version:   "1.0",
			Service:   NewPrivateAccountAPI(apiBackend, nonceLock),
			Public:    false,
		}, {
			Namespace: "neat",
			Version:   "1.0",
			Service:   NewPublicNEATAPI(apiBackend, nonceLock),
			Public:    true,
		},
	}
	return append(compiler, all...)
}
