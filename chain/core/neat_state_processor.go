package core

import (
	"fmt"
	"math/big"

	"github.com/neatlab/neatio/chain/core/state"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/core/vm"
	"github.com/neatlab/neatio/chain/log"
	neatAbi "github.com/neatlab/neatio/neatabi/abi"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/crypto"
)

func ApplyTransactionEx(config *params.ChainConfig, bc *BlockChain, author *common.Address, gp *GasPool, statedb *state.StateDB, ops *types.PendingOps,
	header *types.Header, tx *types.Transaction, usedGas *uint64, totalUsedMoney *big.Int, cfg vm.Config, cch CrossChainHelper, mining bool) (*types.Receipt, uint64, error) {

	signer := types.MakeSigner(config, header.Number)
	msg, err := tx.AsMessage(signer)
	if err != nil {
		return nil, 0, err
	}

	if !neatAbi.IsNeatChainContractAddr(tx.To()) {

		context := NewEVMContext(msg, header, bc, author)

		vmenv := vm.NewEVM(context, statedb, config, cfg)

		_, gas, money, failed, err := ApplyMessageEx(vmenv, msg, gp)
		if err != nil {
			return nil, 0, err
		}

		var root []byte
		if config.IsByzantium(header.Number) {

			statedb.Finalise(true)
		} else {

			root = statedb.IntermediateRoot(false).Bytes()
		}
		*usedGas += gas
		totalUsedMoney.Add(totalUsedMoney, money)

		receipt := types.NewReceipt(root, failed, *usedGas)
		log.Debugf("ApplyTransactionEx，new receipt with (root,failed,*usedGas) = (%v,%v,%v)\n", root, failed, *usedGas)
		receipt.TxHash = tx.Hash()

		receipt.GasUsed = gas

		if msg.To() == nil {
			receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
		}

		receipt.Logs = statedb.GetLogs(tx.Hash())

		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipt.BlockHash = statedb.BlockHash()
		receipt.BlockNumber = header.Number
		receipt.TransactionIndex = uint(statedb.TxIndex())

		return receipt, gas, err

	} else {

		data := tx.Data()
		function, err := neatAbi.FunctionTypeFromId(data[:4])
		if err != nil {
			return nil, 0, err
		}
		log.Infof("ApplyTransactionEx() 0, Chain Function is %v", function.String())

		if config.IsMainChain() && !function.AllowInMainChain() {
			return nil, 0, ErrNotAllowedInMainChain
		} else if !config.IsMainChain() && !function.AllowInSideChain() {
			return nil, 0, ErrNotAllowedInSideChain
		}

		from := msg.From()

		if msg.CheckNonce() {
			nonce := statedb.GetNonce(from)
			if nonce < msg.Nonce() {
				log.Info("ApplyTransactionEx() abort due to nonce too high")
				return nil, 0, ErrNonceTooHigh
			} else if nonce > msg.Nonce() {
				log.Info("ApplyTransactionEx() abort due to nonce too low")
				return nil, 0, ErrNonceTooLow
			}
		}

		gasLimit := tx.Gas()
		gasValue := new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), tx.GasPrice())
		if statedb.GetBalance(from).Cmp(gasValue) < 0 {
			return nil, 0, fmt.Errorf("insufficient NEAT for gas (%x). Req %v, has %v", from.Bytes()[:4], gasValue, statedb.GetBalance(from))
		}
		if err := gp.SubGas(gasLimit); err != nil {
			return nil, 0, err
		}
		statedb.SubBalance(from, gasValue)

		gas := function.RequiredGas()
		if gasLimit < gas {
			return nil, 0, vm.ErrOutOfGas
		}

		if statedb.GetBalance(from).Cmp(tx.Value()) == -1 {
			return nil, 0, fmt.Errorf("insufficient NEAT for tx amount (%x). Req %v, has %v", from.Bytes()[:4], tx.Value(), statedb.GetBalance(from))
		}

		if applyCb := GetApplyCb(function); applyCb != nil {
			if function.IsCrossChainType() {
				if fn, ok := applyCb.(CrossChainApplyCb); ok {
					cch.GetMutex().Lock()
					err := fn(tx, statedb, ops, cch, mining)
					cch.GetMutex().Unlock()

					if err != nil {
						return nil, 0, err
					}
				} else {
					panic("callback func is wrong, this should not happened, please check the code")
				}
			} else {
				if fn, ok := applyCb.(NonCrossChainApplyCb); ok {
					if err := fn(tx, statedb, bc, ops); err != nil {
						return nil, 0, err
					}
				} else {
					panic("callback func is wrong, this should not happened, please check the code")
				}
			}
		}

		remainingGas := gasLimit - gas
		remaining := new(big.Int).Mul(new(big.Int).SetUint64(remainingGas), tx.GasPrice())
		statedb.AddBalance(from, remaining)
		gp.AddGas(remainingGas)

		*usedGas += gas
		totalUsedMoney.Add(totalUsedMoney, new(big.Int).Mul(new(big.Int).SetUint64(gas), tx.GasPrice()))
		log.Infof("ApplyTransactionEx() 2, totalUsedMoney is %v\n", totalUsedMoney)

		var root []byte
		if config.IsByzantium(header.Number) {
			statedb.Finalise(true)
		} else {
			root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
		}
		receipt := types.NewReceipt(root, false, *usedGas)
		receipt.TxHash = tx.Hash()
		receipt.GasUsed = gas

		receipt.Logs = statedb.GetLogs(tx.Hash())
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipt.BlockHash = statedb.BlockHash()
		receipt.BlockNumber = header.Number
		receipt.TransactionIndex = uint(statedb.TxIndex())

		statedb.SetNonce(msg.From(), statedb.GetNonce(msg.From())+1)

		return receipt, 0, nil
	}
}
