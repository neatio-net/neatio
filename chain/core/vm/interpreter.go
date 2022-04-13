package vm

import (
	"fmt"
	"hash"
	"sync/atomic"

	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/utilities/common"

	"github.com/neatlab/neatio/utilities/common/math"
)

type Config struct {
	Debug                   bool
	Tracer                  Tracer
	NoRecursion             bool
	EnablePreimageRecording bool

	JumpTable [256]operation

	EWASMInterpreter string
	EVMInterpreter   string

	ExtraEips []int
}

type Interpreter interface {
	Run(contract *Contract, input []byte, static bool) ([]byte, error)

	CanRun([]byte) bool
}

type keccakState interface {
	hash.Hash
	Read([]byte) (int, error)
}

type EVMInterpreter struct {
	evm *EVM
	cfg Config

	intPool *intPool

	hasher    keccakState
	hasherBuf common.Hash

	readOnly   bool
	returnData []byte
}

func NewEVMInterpreter(evm *EVM, cfg Config) *EVMInterpreter {

	if !cfg.JumpTable[STOP].valid {
		var jt JumpTable
		switch {
		case evm.chainRules.IsIstanbul:
			jt = istanbulInstructionSet
		case evm.chainRules.IsConstantinople:
			jt = constantinopleInstructionSet
		case evm.chainRules.IsByzantium:
			jt = byzantiumInstructionSet
		case evm.chainRules.IsEIP158:
			jt = spuriousDragonInstructionSet
		case evm.chainRules.IsEIP150:
			jt = tangerineWhistleInstructionSet
		case evm.chainRules.IsHomestead:
			jt = homesteadInstructionSet
		default:
			jt = frontierInstructionSet
		}
		for i, eip := range cfg.ExtraEips {
			if err := EnableEIP(eip, &jt); err != nil {

				cfg.ExtraEips = append(cfg.ExtraEips[:i], cfg.ExtraEips[i+1:]...)
				log.Error("EIP activation failed", "eip", eip, "error", err)
			}
		}
		cfg.JumpTable = jt
	}

	return &EVMInterpreter{
		evm: evm,
		cfg: cfg,
	}
}

func (in *EVMInterpreter) Run(contract *Contract, input []byte, readOnly bool) (ret []byte, err error) {
	if in.intPool == nil {
		in.intPool = poolOfIntPools.get()
		defer func() {
			poolOfIntPools.put(in.intPool)
			in.intPool = nil
		}()
	}

	in.evm.depth++
	defer func() { in.evm.depth-- }()

	if readOnly && !in.readOnly {
		in.readOnly = true
		defer func() { in.readOnly = false }()
	}

	in.returnData = nil

	if len(contract.Code) == 0 {
		return nil, nil
	}

	var (
		op    OpCode
		mem   = NewMemory()
		stack = newstack()

		pc   = uint64(0)
		cost uint64

		pcCopy  uint64
		gasCopy uint64
		logged  bool
		res     []byte
	)
	contract.Input = input

	defer func() { in.intPool.put(stack.data...) }()

	if in.cfg.Debug {
		defer func() {
			if err != nil {
				if !logged {
					in.cfg.Tracer.CaptureState(in.evm, pcCopy, op, gasCopy, cost, mem, stack, contract, in.evm.depth, err)
				} else {
					in.cfg.Tracer.CaptureFault(in.evm, pcCopy, op, gasCopy, cost, mem, stack, contract, in.evm.depth, err)
				}
			}
		}()
	}

	for atomic.LoadInt32(&in.evm.abort) == 0 {
		if in.cfg.Debug {

			logged, pcCopy, gasCopy = false, pc, contract.Gas
		}

		op = contract.GetOp(pc)
		operation := in.cfg.JumpTable[op]
		if !operation.valid {
			return nil, fmt.Errorf("invalid opcode 0x%x", int(op))
		}

		if sLen := stack.len(); sLen < operation.minStack {
			return nil, fmt.Errorf("stack underflow (%d <=> %d)", sLen, operation.minStack)
		} else if sLen > operation.maxStack {
			return nil, fmt.Errorf("stack limit reached %d (%d)", sLen, operation.maxStack)
		}

		if in.readOnly && in.evm.chainRules.IsByzantium {

			if operation.writes || (op == CALL && stack.Back(2).Sign() != 0) {
				return nil, ErrWriteProtection
			}
		}

		cost = operation.constantGas
		if !contract.UseGas(operation.constantGas) {
			return nil, ErrOutOfGas
		}

		var memorySize uint64

		if operation.memorySize != nil {
			memSize, overflow := operation.memorySize(stack)
			if overflow {
				return nil, errGasUintOverflow
			}

			if memorySize, overflow = math.SafeMul(toWordSize(memSize), 32); overflow {
				return nil, errGasUintOverflow
			}
		}

		if operation.dynamicGas != nil {
			var dynamicCost uint64
			dynamicCost, err = operation.dynamicGas(in.evm, contract, stack, mem, memorySize)
			cost += dynamicCost
			if err != nil || !contract.UseGas(dynamicCost) {
				return nil, ErrOutOfGas
			}
		}
		if memorySize > 0 {
			mem.Resize(memorySize)
		}

		if in.cfg.Debug {
			in.cfg.Tracer.CaptureState(in.evm, pc, op, gasCopy, cost, mem, stack, contract, in.evm.depth, err)
			logged = true
		}

		res, err = operation.execute(&pc, in, contract, mem, stack)

		if verifyPool {
			verifyIntegerPool(in.intPool)
		}

		if operation.returns {
			in.returnData = res
		}

		switch {
		case err != nil:
			return nil, err
		case operation.reverts:
			return res, ErrExecutionReverted
		case operation.halts:
			return res, nil
		case !operation.jumps:
			pc++
		}
	}
	return nil, nil
}

func (in *EVMInterpreter) CanRun(code []byte) bool {
	return true
}
