package neatcli

import "github.com/neatlab/neatio"

var (
	_ = neatio.ChainReader(&Client{})
	_ = neatio.TransactionReader(&Client{})
	_ = neatio.ChainStateReader(&Client{})
	_ = neatio.ChainSyncReader(&Client{})
	_ = neatio.ContractCaller(&Client{})
	_ = neatio.GasEstimator(&Client{})
	_ = neatio.GasPricer(&Client{})
	_ = neatio.LogFilterer(&Client{})
	_ = neatio.PendingStateReader(&Client{})

	_ = neatio.PendingContractCaller(&Client{})
)
