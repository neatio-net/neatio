package core

import (
	"math/big"

	"github.com/neatlab/neatio/chain/core/vm"
	"github.com/neatlab/neatio/chain/log"
)

func ApplyMessageEx(evm *vm.EVM, msg Message, gp *GasPool) ([]byte, uint64, *big.Int, bool, error) {
	return NewStateTransition(evm, msg, gp).TransitionDbEx()
}

func (st *StateTransition) TransitionDbEx() (ret []byte, usedGas uint64, usedMoney *big.Int, failed bool, err error) {

	if err = st.preCheck(); err != nil {
		return
	}
	msg := st.msg
	sender := st.from()

	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)
	contractCreation := msg.To() == nil

	gas, err := IntrinsicGas(st.data, contractCreation, homestead)
	if err != nil {
		return nil, 0, nil, false, err
	}
	if err = st.useGas(gas); err != nil {
		return nil, 0, nil, false, err
	}

	var (
		evm = st.evm

		vmerr error
	)

	if contractCreation {
		ret, _, st.gas, vmerr = evm.Create(sender, st.data, st.gas, st.value)
	} else {

		st.state.SetNonce(sender.Address(), st.state.GetNonce(sender.Address())+1)
		ret, st.gas, vmerr = evm.Call(sender, st.to().Address(), st.data, st.gas, st.value)

	}

	if vmerr != nil {
		log.Debug("VM returned with error", "err", vmerr)

		if vmerr == vm.ErrInsufficientBalance {
			return nil, 0, nil, false, vmerr
		}
	}

	st.refundGas()

	usedMoney = new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice)

	return ret, st.gasUsed(), usedMoney, vmerr != nil, err
}
