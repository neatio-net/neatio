package core

import (
	"fmt"
	"math/big"

	"github.com/neatio-network/neatio/chain/consensus"
	"github.com/neatio-network/neatio/chain/core/state"
	"github.com/neatio-network/neatio/chain/core/types"
	"github.com/neatio-network/neatio/chain/core/vm"
	"github.com/neatio-network/neatio/chain/log"
	neatAbi "github.com/neatio-network/neatio/neatabi/abi"
	"github.com/neatio-network/neatio/params"
	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/crypto"
)

type StateProcessor struct {
	config *params.ChainConfig
	bc     *BlockChain
	engine consensus.Engine
	cch    CrossChainHelper
}

func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine, cch CrossChainHelper) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
		cch:    cch,
	}
}

func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, *types.PendingOps, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
		ops      = new(types.PendingOps)
	)

	totalUsedMoney := big.NewInt(0)

	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)

		receipt, err := ApplyTransactionEx(p.config, p.bc, nil, gp, statedb, ops, header, tx,
			usedGas, totalUsedMoney, cfg, p.cch, false)
		log.Debugf("(p *StateProcessor) Process()，after ApplyTransactionEx, receipt is %v\n", receipt)
		if err != nil {
			return nil, nil, 0, nil, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}

	_, err := p.engine.Finalize(p.bc, header, statedb, block.Transactions(), totalUsedMoney, block.Uncles(), receipts, ops)
	if err != nil {
		return nil, nil, 0, nil, err
	}

	return receipts, allLogs, *usedGas, ops, nil
}

func ApplyTransaction(config *params.ChainConfig, bc *BlockChain, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, err
	}

	context := NewEVMContext(msg, header, bc, author)

	vmenv := vm.NewEVM(context, statedb, config, cfg)

	result, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, err
	}

	var root []byte
	if config.IsByzantium(header.Number) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += result.UsedGas

	receipt := types.NewReceipt(root, result.Failed(), *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas

	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}

	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	return receipt, err
}

func ApplyTransactionEx(config *params.ChainConfig, bc *BlockChain, author *common.Address, gp *GasPool, statedb *state.StateDB, ops *types.PendingOps,
	header *types.Header, tx *types.Transaction, usedGas *uint64, totalUsedMoney *big.Int, cfg vm.Config, cch CrossChainHelper, mining bool) (*types.Receipt, error) {

	signer := types.MakeSigner(config, header.Number)
	msg, err := tx.AsMessage(signer)
	if err != nil {
		return nil, err
	}

	if !neatAbi.IsNeatChainContractAddr(tx.To()) {

		context := NewEVMContext(msg, header, bc, author)

		vmenv := vm.NewEVM(context, statedb, config, cfg)

		result, money, err := ApplyMessageEx(vmenv, msg, gp)
		if err != nil {
			return nil, err
		}

		var root []byte
		if config.IsByzantium(header.Number) {

			statedb.Finalise(true)
		} else {

			root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
		}
		*usedGas += result.UsedGas
		totalUsedMoney.Add(totalUsedMoney, money)

		receipt := types.NewReceipt(root, result.Failed(), *usedGas)
		log.Debugf("ApplyTransactionEx，new receipt with (root,failed,*usedGas) = (%v,%v,%v)\n", root, result.Failed(), *usedGas)
		receipt.TxHash = tx.Hash()

		receipt.GasUsed = result.UsedGas

		if msg.To() == nil {
			receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
		}

		receipt.Logs = statedb.GetLogs(tx.Hash())

		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipt.BlockHash = statedb.BlockHash()
		receipt.BlockNumber = header.Number
		receipt.TransactionIndex = uint(statedb.TxIndex())

		return receipt, err

	} else {

		data := tx.Data()
		function, err := neatAbi.FunctionTypeFromId(data[:4])
		if err != nil {
			return nil, err
		}
		log.Infof("ApplyTransactionEx() 0, Chain Function is %v", function.String())

		if config.IsMainChain() && !function.AllowInMainChain() {
			return nil, ErrNotAllowedInMainChain
		} else if !config.IsMainChain() && !function.AllowInSideChain() {
			return nil, ErrNotAllowedInSideChain
		}

		from := msg.From()

		if msg.CheckNonce() {
			nonce := statedb.GetNonce(from)
			if nonce < msg.Nonce() {
				log.Info("ApplyTransactionEx() abort due to nonce too high")
				return nil, ErrNonceTooHigh
			} else if nonce > msg.Nonce() {
				log.Info("ApplyTransactionEx() abort due to nonce too low")
				return nil, ErrNonceTooLow
			}
		}

		gasLimit := tx.Gas()
		gasValue := new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), tx.GasPrice())
		if statedb.GetBalance(from).Cmp(gasValue) < 0 {
			return nil, fmt.Errorf("insufficient NEAT for gas (%x). Req %v, has %v", from.Bytes()[:4], gasValue, statedb.GetBalance(from))
		}
		if err := gp.SubGas(gasLimit); err != nil {
			return nil, err
		}
		statedb.SubBalance(from, gasValue)

		gas := function.RequiredGas()
		if gasLimit < gas {
			return nil, vm.ErrOutOfGas
		}

		if statedb.GetBalance(from).Cmp(tx.Value()) == -1 {
			return nil, fmt.Errorf("insufficient NEAT for tx amount (%x). Req %v, has %v", from.Bytes()[:4], tx.Value(), statedb.GetBalance(from))
		}

		if applyCb := GetApplyCb(function); applyCb != nil {
			if function.IsCrossChainType() {
				if fn, ok := applyCb.(CrossChainApplyCb); ok {
					cch.GetMutex().Lock()
					err := fn(tx, statedb, ops, cch, mining)
					cch.GetMutex().Unlock()

					if err != nil {
						return nil, err
					}
				} else {
					panic("callback func is wrong, this should not happened, please check the code")
				}
			} else {
				if fn, ok := applyCb.(NonCrossChainApplyCb); ok {
					if err := fn(tx, statedb, bc, ops); err != nil {
						return nil, err
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

		return receipt, nil
	}
}
