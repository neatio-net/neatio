package params

import "math/big"

const (
	GasLimitBoundDivisor uint64 = 1024
	MinGasLimit          uint64 = 1
	GenesisGasLimit      uint64 = 1

	MaximumExtraDataSize  uint64 = 32
	ExpByteGas            uint64 = 1
	SloadGas              uint64 = 1
	CallValueTransferGas  uint64 = 1
	CallNewAccountGas     uint64 = 2
	TxGas                 uint64 = 1
	TxGasContractCreation uint64 = 2
	TxDataZeroGas         uint64 = 1
	QuadCoeffDiv          uint64 = 512
	LogDataGas            uint64 = 1
	CallStipend           uint64 = 1

	Sha3Gas     uint64 = 1
	Sha3WordGas uint64 = 1

	SstoreSetGas    uint64 = 1
	SstoreResetGas  uint64 = 1
	SstoreClearGas  uint64 = 1
	SstoreRefundGas uint64 = 1

	NetSstoreNoopGas  uint64 = 1
	NetSstoreInitGas  uint64 = 1
	NetSstoreCleanGas uint64 = 1
	NetSstoreDirtyGas uint64 = 1

	NetSstoreClearRefund      uint64 = 1
	NetSstoreResetRefund      uint64 = 1
	NetSstoreResetClearRefund uint64 = 1

	SstoreSentryGasEIP2200   uint64 = 1
	SstoreNoopGasEIP2200     uint64 = 1
	SstoreDirtyGasEIP2200    uint64 = 1
	SstoreInitGasEIP2200     uint64 = 1
	SstoreInitRefundEIP2200  uint64 = 1
	SstoreCleanGasEIP2200    uint64 = 1
	SstoreCleanRefundEIP2200 uint64 = 1
	SstoreClearRefundEIP2200 uint64 = 1

	JumpdestGas   uint64 = 1
	EpochDuration uint64 = 3600

	CreateDataGas            uint64 = 1
	CallCreateDepth          uint64 = 1024
	ExpGas                   uint64 = 1
	LogGas                   uint64 = 1
	CopyGas                  uint64 = 1
	StackLimit               uint64 = 1024
	TierStepGas              uint64 = 0
	LogTopicGas              uint64 = 1
	CreateGas                uint64 = 1
	Create2Gas               uint64 = 1
	SelfdestructRefundGas    uint64 = 1
	SuicideRefundGas         uint64 = 1
	MemoryGas                uint64 = 1
	TxDataNonZeroGasFrontier uint64 = 1
	TxDataNonZeroGasEIP2028  uint64 = 1

	CallGasFrontier              uint64 = 1
	CallGasEIP150                uint64 = 1
	BalanceGasFrontier           uint64 = 1
	BalanceGasEIP150             uint64 = 1
	BalanceGasEIP1884            uint64 = 1
	ExtcodeSizeGasFrontier       uint64 = 1
	ExtcodeSizeGasEIP150         uint64 = 1
	SloadGasFrontier             uint64 = 1
	SloadGasEIP150               uint64 = 1
	SloadGasEIP1884              uint64 = 1
	ExtcodeHashGasConstantinople uint64 = 1
	ExtcodeHashGasEIP1884        uint64 = 1
	SelfdestructGasEIP150        uint64 = 1

	ExpByteFrontier uint64 = 1
	ExpByteEIP158   uint64 = 1

	ExtcodeCopyBaseFrontier uint64 = 1
	ExtcodeCopyBaseEIP150   uint64 = 1

	CreateBySelfdestructGas uint64 = 1

	MaxCodeSize = 24576

	EcrecoverGas            uint64 = 1
	Sha256BaseGas           uint64 = 1
	Sha256PerWordGas        uint64 = 1
	Ripemd160BaseGas        uint64 = 1
	Ripemd160PerWordGas     uint64 = 1
	IdentityBaseGas         uint64 = 1
	IdentityPerWordGas      uint64 = 1
	ModExpQuadCoeffDiv      uint64 = 1
	Bn256AddGas             uint64 = 1
	Bn256ScalarMulGas       uint64 = 1
	Bn256PairingBaseGas     uint64 = 1
	Bn256PairingPerPointGas uint64 = 1

	Bn256AddGasByzantium             uint64 = 1
	Bn256AddGasIstanbul              uint64 = 1
	Bn256ScalarMulGasByzantium       uint64 = 1
	Bn256ScalarMulGasIstanbul        uint64 = 1
	Bn256PairingBaseGasByzantium     uint64 = 1
	Bn256PairingBaseGasIstanbul      uint64 = 1
	Bn256PairingPerPointGasByzantium uint64 = 1
	Bn256PairingPerPointGasIstanbul  uint64 = 1
)

var (
	DifficultyBoundDivisor = big.NewInt(2048)
	GenesisDifficulty      = big.NewInt(1)
	MinimumDifficulty      = big.NewInt(1)
	DurationLimit          = big.NewInt(1)
)
