package bind

import (
	"context"
	"errors"
	"math/big"

	"github.com/neatio-net/neatio"
	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/utilities/common"
)

var (
	ErrNoCode = errors.New("no contract code at given address")

	ErrNoPendingState = errors.New("backend does not support pending state")

	ErrNoCodeAfterDeploy = errors.New("no contract code after deployment")
)

type ContractCaller interface {
	CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error)

	CallContract(ctx context.Context, call neatio.CallMsg, blockNumber *big.Int) ([]byte, error)
}

type PendingContractCaller interface {
	PendingCodeAt(ctx context.Context, contract common.Address) ([]byte, error)

	PendingCallContract(ctx context.Context, call neatio.CallMsg) ([]byte, error)
}

type ContractTransactor interface {
	PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error)

	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)

	SuggestGasPrice(ctx context.Context) (*big.Int, error)

	EstimateGas(ctx context.Context, call neatio.CallMsg) (gas uint64, err error)

	SendTransaction(ctx context.Context, tx *types.Transaction) error
}

type ContractFilterer interface {
	FilterLogs(ctx context.Context, query neatio.FilterQuery) ([]types.Log, error)

	SubscribeFilterLogs(ctx context.Context, query neatio.FilterQuery, ch chan<- types.Log) (neatio.Subscription, error)
}

type DeployBackend interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)
}

type ContractBackend interface {
	ContractCaller
	ContractTransactor
	ContractFilterer
}
