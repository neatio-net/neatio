package vm

import (
	"fmt"

	"github.com/neatio-network/neatio/params"
)

func EnableEIP(eipNum int, jt *JumpTable) error {
	switch eipNum {
	case 2200:
		enable2200(jt)
	case 1884:
		enable1884(jt)
	case 1344:
		enable1344(jt)
	default:
		return fmt.Errorf("undefined eip %d", eipNum)
	}
	return nil
}

func enable1884(jt *JumpTable) {

	jt[BALANCE].constantGas = params.BalanceGasEIP1884
	jt[EXTCODEHASH].constantGas = params.ExtcodeHashGasEIP1884
	jt[SLOAD].constantGas = params.SloadGasEIP1884

	jt[SELFBALANCE] = operation{
		execute:     opSelfBalance,
		constantGas: GasFastStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
		valid:       true,
	}
}

func opSelfBalance(pc *uint64, interpreter *EVMInterpreter, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	balance := interpreter.intPool.get().Set(interpreter.evm.StateDB.GetBalance(contract.Address()))
	stack.push(balance)
	return nil, nil
}

func enable1344(jt *JumpTable) {

	jt[CHAINID] = operation{
		execute:     opChainID,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
		valid:       true,
	}
}

func opChainID(pc *uint64, interpreter *EVMInterpreter, contract *Contract, memory *Memory, stack *Stack) ([]byte, error) {
	chainId := interpreter.intPool.get().Set(interpreter.evm.chainConfig.ChainId)
	stack.push(chainId)
	return nil, nil
}

func enable2200(jt *JumpTable) {
	jt[SSTORE].dynamicGas = gasSStoreEIP2200
}
