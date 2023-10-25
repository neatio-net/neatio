package core

import (
	"math/big"

	"github.com/nio-net/nio/chain/consensus"
	"github.com/nio-net/nio/chain/core/state"
	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/core/vm"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/params"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/crypto"
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
		receipt, _, err := ApplyTransactionEx(p.config, p.bc, nil, gp, statedb, ops, header, tx,
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

func ApplyTransaction(config *params.ChainConfig, bc *BlockChain, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, uint64, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}
	context := NewEVMContext(msg, header, bc, author)
	vmenv := vm.NewEVM(context, statedb, config, cfg)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, 0, err
	}
	var root []byte
	if config.IsByzantium(header.Number) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gas

	receipt := types.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	return receipt, gas, err
}
