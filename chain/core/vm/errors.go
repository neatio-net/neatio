package vm

import "errors"

var (
	ErrOutOfGas                 = errors.New("out of gas")
	ErrCodeStoreOutOfGas        = errors.New("contract creation code storage out of gas")
	ErrDepth                    = errors.New("max call depth exceeded")
	ErrTraceLimitReached        = errors.New("the number of logs reached the specified limit")
	ErrInsufficientBalance      = errors.New("insufficient balance for transfer")
	ErrContractAddressCollision = errors.New("contract address collision")
	ErrNoCompatibleInterpreter  = errors.New("no compatible interpreter")

	ErrWriteProtection       = errors.New("write protection")
	ErrReturnDataOutOfBounds = errors.New("return data out of bounds")
	ErrExecutionReverted     = errors.New("execution reverted")
	ErrMaxCodeSizeExceeded   = errors.New("max code size exceeded")
	ErrInvalidJump           = errors.New("invalid jump destination")
)
