package neatptc

import (
	"fmt"
	"io"
	"math/big"

	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/event"
	"github.com/neatlab/neatio/utilities/rlp"
)

const (
	intprotocol63 = 63
	intprotocol64 = 64
	intprotocol65 = 65
)

const protocolName = "neatptc"

var ProtocolVersions = []uint{intprotocol65, intprotocol64, intprotocol63}

var protocolLengths = map[uint]uint64{intprotocol65: 17, intprotocol64: 17, intprotocol63: 17}

const ProtocolMaxMsgSize = 10 * 1024 * 1024

const (
	StatusMsg          = 0x00
	NewBlockHashesMsg  = 0x01
	TxMsg              = 0x02
	GetBlockHeadersMsg = 0x03
	BlockHeadersMsg    = 0x04
	GetBlockBodiesMsg  = 0x05
	BlockBodiesMsg     = 0x06
	NewBlockMsg        = 0x07

	GetNodeDataMsg = 0x0d
	NodeDataMsg    = 0x0e
	GetReceiptsMsg = 0x0f
	ReceiptsMsg    = 0x10

	TX3ProofDataMsg = 0x18

	GetPreImagesMsg = 0x19
	PreImagesMsg    = 0x1a
	TrieNodeDataMsg = 0x1b
)

type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIdMismatch
	ErrGenesisBlockMismatch
	ErrNoStatusMsg
	ErrExtraStatusMsg
	ErrSuspendedPeer
	ErrTX3ValidateFail
)

func (e errCode) String() string {
	return errorToString[int(e)]
}

var errorToString = map[int]string{
	ErrMsgTooLarge:             "Message too long",
	ErrDecode:                  "Invalid message",
	ErrInvalidMsgCode:          "Invalid message code",
	ErrProtocolVersionMismatch: "Protocol version mismatch",
	ErrNetworkIdMismatch:       "NetworkId mismatch",
	ErrGenesisBlockMismatch:    "Genesis block mismatch",
	ErrNoStatusMsg:             "No status message",
	ErrExtraStatusMsg:          "Extra status message",
	ErrSuspendedPeer:           "Suspended peer",
	ErrTX3ValidateFail:         "TX3 validate fail",
}

type txPool interface {
	AddRemotes([]*types.Transaction) []error

	Pending() (map[common.Address]types.Transactions, error)

	SubscribeTxPreEvent(chan<- core.TxPreEvent) event.Subscription
}

type statusData struct {
	ProtocolVersion uint32
	NetworkId       uint64
	TD              *big.Int
	CurrentBlock    common.Hash
	GenesisBlock    common.Hash
}

type newBlockHashesData []struct {
	Hash   common.Hash
	Number uint64
}

type getBlockHeadersData struct {
	Origin  hashOrNumber
	Amount  uint64
	Skip    uint64
	Reverse bool
}

type hashOrNumber struct {
	Hash   common.Hash
	Number uint64
}

func (hn *hashOrNumber) EncodeRLP(w io.Writer) error {
	if hn.Hash == (common.Hash{}) {
		return rlp.Encode(w, hn.Number)
	}
	if hn.Number != 0 {
		return fmt.Errorf("both origin hash (%x) and number (%d) provided", hn.Hash, hn.Number)
	}
	return rlp.Encode(w, hn.Hash)
}

func (hn *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	origin, err := s.Raw()
	if err == nil {
		switch {
		case size == 32:
			err = rlp.DecodeBytes(origin, &hn.Hash)
		case size <= 8:
			err = rlp.DecodeBytes(origin, &hn.Number)
		default:
			err = fmt.Errorf("invalid input size %d for origin", size)
		}
	}
	return err
}

type newBlockData struct {
	Block *types.Block
	TD    *big.Int
}

type blockBody struct {
	Transactions []*types.Transaction
	Uncles       []*types.Header
}

type blockBodiesData []*blockBody
