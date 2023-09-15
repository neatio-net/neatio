package params

import "math/big"

const (
	GasLimitBoundDivisor uint64 = 1024
	MinGasLimit          uint64 = 5000
	GenesisGasLimit      uint64 = 4712388

	MaximumExtraDataSize  uint64 = 32
	ExpByteGas            uint64 = 10
	SloadGas              uint64 = 50
	CallValueTransferGas  uint64 = 9000
	CallNewAccountGas     uint64 = 25000
	TxGas                 uint64 = 21000
	TxGasContractCreation uint64 = 53000
	TxDataZeroGas         uint64 = 4
	QuadCoeffDiv          uint64 = 512
	LogDataGas            uint64 = 8
	CallStipend           uint64 = 2300

	Sha3Gas     uint64 = 30
	Sha3WordGas uint64 = 6

	SstoreSetGas    uint64 = 20000
	SstoreResetGas  uint64 = 5000
	SstoreClearGas  uint64 = 5000
	SstoreRefundGas uint64 = 15000

	NetSstoreNoopGas  uint64 = 200
	NetSstoreInitGas  uint64 = 20000
	NetSstoreCleanGas uint64 = 5000
	NetSstoreDirtyGas uint64 = 200

	NetSstoreClearRefund      uint64 = 15000
	NetSstoreResetRefund      uint64 = 4800
	NetSstoreResetClearRefund uint64 = 19800

	SstoreSentryGasEIP2200   uint64 = 2300
	SstoreNoopGasEIP2200     uint64 = 800
	SstoreDirtyGasEIP2200    uint64 = 800
	SstoreInitGasEIP2200     uint64 = 20000
	SstoreInitRefundEIP2200  uint64 = 19200
	SstoreCleanGasEIP2200    uint64 = 5000
	SstoreCleanRefundEIP2200 uint64 = 4200
	SstoreClearRefundEIP2200 uint64 = 15000

	JumpdestGas   uint64 = 1
	EpochDuration uint64 = 86457

	CreateDataGas            uint64 = 200
	CallCreateDepth          uint64 = 1024
	ExpGas                   uint64 = 10
	LogGas                   uint64 = 375
	CopyGas                  uint64 = 3
	StackLimit               uint64 = 1024
	TierStepGas              uint64 = 0
	LogTopicGas              uint64 = 375
	CreateGas                uint64 = 32000
	Create2Gas               uint64 = 32000
	SelfdestructRefundGas    uint64 = 24000
	SuicideRefundGas         uint64 = 24000
	MemoryGas                uint64 = 3
	TxDataNonZeroGasFrontier uint64 = 68
	TxDataNonZeroGasEIP2028  uint64 = 16

	CallGasFrontier              uint64 = 40
	CallGasEIP150                uint64 = 700
	BalanceGasFrontier           uint64 = 20
	BalanceGasEIP150             uint64 = 400
	BalanceGasEIP1884            uint64 = 700
	ExtcodeSizeGasFrontier       uint64 = 20
	ExtcodeSizeGasEIP150         uint64 = 700
	SloadGasFrontier             uint64 = 50
	SloadGasEIP150               uint64 = 200
	SloadGasEIP1884              uint64 = 800
	ExtcodeHashGasConstantinople uint64 = 400
	ExtcodeHashGasEIP1884        uint64 = 700
	SelfdestructGasEIP150        uint64 = 5000

	ExpByteFrontier uint64 = 10
	ExpByteEIP158   uint64 = 50

	ExtcodeCopyBaseFrontier uint64 = 20
	ExtcodeCopyBaseEIP150   uint64 = 700

	CreateBySelfdestructGas uint64 = 25000

	MaxCodeSize = 24576

	EcrecoverGas            uint64 = 3000
	Sha256BaseGas           uint64 = 60
	Sha256PerWordGas        uint64 = 12
	Ripemd160BaseGas        uint64 = 600
	Ripemd160PerWordGas     uint64 = 120
	IdentityBaseGas         uint64 = 15
	IdentityPerWordGas      uint64 = 3
	ModExpQuadCoeffDiv      uint64 = 20
	Bn256AddGas             uint64 = 500
	Bn256ScalarMulGas       uint64 = 40000
	Bn256PairingBaseGas     uint64 = 100000
	Bn256PairingPerPointGas uint64 = 80000

	Bn256AddGasByzantium             uint64 = 500
	Bn256AddGasIstanbul              uint64 = 150
	Bn256ScalarMulGasByzantium       uint64 = 40000
	Bn256ScalarMulGasIstanbul        uint64 = 6000
	Bn256PairingBaseGasByzantium     uint64 = 100000
	Bn256PairingBaseGasIstanbul      uint64 = 45000
	Bn256PairingPerPointGasByzantium uint64 = 80000
	Bn256PairingPerPointGasIstanbul  uint64 = 34000
)

var (
	DifficultyBoundDivisor = big.NewInt(2048)
	GenesisDifficulty      = big.NewInt(131072)
	MinimumDifficulty      = big.NewInt(131072)
	DurationLimit          = big.NewInt(13)
)
