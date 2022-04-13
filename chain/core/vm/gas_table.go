package vm

import (
	"errors"

	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/math"
)

func memoryGasCost(mem *Memory, newMemSize uint64) (uint64, error) {
	if newMemSize == 0 {
		return 0, nil
	}

	if newMemSize > 0x1FFFFFFFE0 {
		return 0, errGasUintOverflow
	}
	newMemSizeWords := toWordSize(newMemSize)
	newMemSize = newMemSizeWords * 32

	if newMemSize > uint64(mem.Len()) {
		square := newMemSizeWords * newMemSizeWords
		linCoef := newMemSizeWords * params.MemoryGas
		quadCoef := square / params.QuadCoeffDiv
		newTotalFee := linCoef + quadCoef

		fee := newTotalFee - mem.lastGasCost
		mem.lastGasCost = newTotalFee

		return fee, nil
	}
	return 0, nil
}

func memoryCopierGas(stackpos int) gasFunc {
	return func(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {

		gas, err := memoryGasCost(mem, memorySize)
		if err != nil {
			return 0, err
		}

		words, overflow := bigUint64(stack.Back(stackpos))
		if overflow {
			return 0, errGasUintOverflow
		}

		if words, overflow = math.SafeMul(toWordSize(words), params.CopyGas); overflow {
			return 0, errGasUintOverflow
		}

		if gas, overflow = math.SafeAdd(gas, words); overflow {
			return 0, errGasUintOverflow
		}
		return gas, nil
	}
}

var (
	gasCallDataCopy   = memoryCopierGas(2)
	gasCodeCopy       = memoryCopierGas(2)
	gasExtCodeCopy    = memoryCopierGas(3)
	gasReturnDataCopy = memoryCopierGas(2)
)

func gasSStore(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	var (
		y, x    = stack.Back(1), stack.Back(0)
		current = evm.StateDB.GetState(contract.Address(), common.BigToHash(x))
	)

	if evm.chainRules.IsPetersburg || !evm.chainRules.IsConstantinople {

		switch {
		case current == (common.Hash{}) && y.Sign() != 0:
			return params.SstoreSetGas, nil
		case current != (common.Hash{}) && y.Sign() == 0:
			evm.StateDB.AddRefund(params.SstoreRefundGas)
			return params.SstoreClearGas, nil
		default:
			return params.SstoreResetGas, nil
		}
	}

	value := common.BigToHash(y)
	if current == value {
		return params.NetSstoreNoopGas, nil
	}
	original := evm.StateDB.GetCommittedState(contract.Address(), common.BigToHash(x))
	if original == current {
		if original == (common.Hash{}) {
			return params.NetSstoreInitGas, nil
		}
		if value == (common.Hash{}) {
			evm.StateDB.AddRefund(params.NetSstoreClearRefund)
		}
		return params.NetSstoreCleanGas, nil
	}
	if original != (common.Hash{}) {
		if current == (common.Hash{}) {
			evm.StateDB.SubRefund(params.NetSstoreClearRefund)
		} else if value == (common.Hash{}) {
			evm.StateDB.AddRefund(params.NetSstoreClearRefund)
		}
	}
	if original == value {
		if original == (common.Hash{}) {
			evm.StateDB.AddRefund(params.NetSstoreResetClearRefund)
		} else {
			evm.StateDB.AddRefund(params.NetSstoreResetRefund)
		}
	}
	return params.NetSstoreDirtyGas, nil
}

func gasSStoreEIP2200(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {

	if contract.Gas <= params.SstoreSentryGasEIP2200 {
		return 0, errors.New("not enough gas for reentrancy sentry")
	}

	var (
		y, x    = stack.Back(1), stack.Back(0)
		current = evm.StateDB.GetState(contract.Address(), common.BigToHash(x))
	)
	value := common.BigToHash(y)

	if current == value {
		return params.SstoreNoopGasEIP2200, nil
	}
	original := evm.StateDB.GetCommittedState(contract.Address(), common.BigToHash(x))
	if original == current {
		if original == (common.Hash{}) {
			return params.SstoreInitGasEIP2200, nil
		}
		if value == (common.Hash{}) {
			evm.StateDB.AddRefund(params.SstoreClearRefundEIP2200)
		}
		return params.SstoreCleanGasEIP2200, nil
	}
	if original != (common.Hash{}) {
		if current == (common.Hash{}) {
			evm.StateDB.SubRefund(params.SstoreClearRefundEIP2200)
		} else if value == (common.Hash{}) {
			evm.StateDB.AddRefund(params.SstoreClearRefundEIP2200)
		}
	}
	if original == value {
		if original == (common.Hash{}) {
			evm.StateDB.AddRefund(params.SstoreInitRefundEIP2200)
		} else {
			evm.StateDB.AddRefund(params.SstoreCleanRefundEIP2200)
		}
	}
	return params.SstoreDirtyGasEIP2200, nil
}

func makeGasLog(n uint64) gasFunc {
	return func(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
		requestedSize, overflow := bigUint64(stack.Back(1))
		if overflow {
			return 0, errGasUintOverflow
		}

		gas, err := memoryGasCost(mem, memorySize)
		if err != nil {
			return 0, err
		}

		if gas, overflow = math.SafeAdd(gas, params.LogGas); overflow {
			return 0, errGasUintOverflow
		}
		if gas, overflow = math.SafeAdd(gas, n*params.LogTopicGas); overflow {
			return 0, errGasUintOverflow
		}

		var memorySizeGas uint64
		if memorySizeGas, overflow = math.SafeMul(requestedSize, params.LogDataGas); overflow {
			return 0, errGasUintOverflow
		}
		if gas, overflow = math.SafeAdd(gas, memorySizeGas); overflow {
			return 0, errGasUintOverflow
		}
		return gas, nil
	}
}

func gasSha3(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	wordGas, overflow := bigUint64(stack.Back(1))
	if overflow {
		return 0, errGasUintOverflow
	}
	if wordGas, overflow = math.SafeMul(toWordSize(wordGas), params.Sha3WordGas); overflow {
		return 0, errGasUintOverflow
	}
	if gas, overflow = math.SafeAdd(gas, wordGas); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func pureMemoryGascost(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	return memoryGasCost(mem, memorySize)
}

var (
	gasReturn  = pureMemoryGascost
	gasRevert  = pureMemoryGascost
	gasMLoad   = pureMemoryGascost
	gasMStore8 = pureMemoryGascost
	gasMStore  = pureMemoryGascost
	gasCreate  = pureMemoryGascost
)

func gasCreate2(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	wordGas, overflow := bigUint64(stack.Back(2))
	if overflow {
		return 0, errGasUintOverflow
	}
	if wordGas, overflow = math.SafeMul(toWordSize(wordGas), params.Sha3WordGas); overflow {
		return 0, errGasUintOverflow
	}
	if gas, overflow = math.SafeAdd(gas, wordGas); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasExpFrontier(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	expByteLen := uint64((stack.data[stack.len()-2].BitLen() + 7) / 8)

	var (
		gas      = expByteLen * params.ExpByteFrontier
		overflow bool
	)
	if gas, overflow = math.SafeAdd(gas, params.ExpGas); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasExpEIP158(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	expByteLen := uint64((stack.data[stack.len()-2].BitLen() + 7) / 8)

	var (
		gas      = expByteLen * params.ExpByteEIP158
		overflow bool
	)
	if gas, overflow = math.SafeAdd(gas, params.ExpGas); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasCall(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	var (
		gas            uint64
		transfersValue = stack.Back(2).Sign() != 0
		address        = common.BigToAddress(stack.Back(1))
	)
	if evm.chainRules.IsEIP158 {
		if transfersValue && evm.StateDB.Empty(address) {
			gas += params.CallNewAccountGas
		}
	} else if !evm.StateDB.Exist(address) {
		gas += params.CallNewAccountGas
	}
	if transfersValue {
		gas += params.CallValueTransferGas
	}
	memoryGas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, memoryGas); overflow {
		return 0, errGasUintOverflow
	}

	evm.callGasTemp, err = callGas(evm.chainRules.IsEIP150, contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	if gas, overflow = math.SafeAdd(gas, evm.callGasTemp); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasCallCode(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	memoryGas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	var (
		gas      uint64
		overflow bool
	)
	if stack.Back(2).Sign() != 0 {
		gas += params.CallValueTransferGas
	}
	if gas, overflow = math.SafeAdd(gas, memoryGas); overflow {
		return 0, errGasUintOverflow
	}
	evm.callGasTemp, err = callGas(evm.chainRules.IsEIP150, contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	if gas, overflow = math.SafeAdd(gas, evm.callGasTemp); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasDelegateCall(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	evm.callGasTemp, err = callGas(evm.chainRules.IsEIP150, contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, evm.callGasTemp); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasStaticCall(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	evm.callGasTemp, err = callGas(evm.chainRules.IsEIP150, contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, evm.callGasTemp); overflow {
		return 0, errGasUintOverflow
	}
	return gas, nil
}

func gasSelfdestruct(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	var gas uint64

	if evm.chainRules.IsEIP150 {
		gas = params.SelfdestructGasEIP150
		var address = common.BigToAddress(stack.Back(0))

		if evm.chainRules.IsEIP158 {

			if evm.StateDB.Empty(address) && evm.StateDB.GetBalance(contract.Address()).Sign() != 0 {
				gas += params.CreateBySelfdestructGas
			}
		} else if !evm.StateDB.Exist(address) {
			gas += params.CreateBySelfdestructGas
		}
	}

	if !evm.StateDB.HasSuicided(contract.Address()) {
		evm.StateDB.AddRefund(params.SelfdestructRefundGas)
	}
	return gas, nil
}
