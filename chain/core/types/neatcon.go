package types

import (
	"errors"
	"math/big"

	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/common/hexutil"
)

var (
	NeatConDigest = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	NeatConExtraVanity = 32
	NeatConExtraSeal   = 65

	NeatConDefaultDifficulty = big.NewInt(1)
	NeatConNilUncleHash      = CalcUncleHash(nil)
	NeatConEmptyNonce        = BlockNonce{}
	NeatConNonce             = hexutil.MustDecode("0x88ff88ff88ff88ff")

	MagicExtra = []byte("neatio_tmp_extra")

	ErrInvalidNeatConHeaderExtra = errors.New("invalid ipbft header extra-data")
)

func NeatConFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	payload := MagicExtra
	newHeader.Extra = payload
	return newHeader
}

func NeatConFilteredHeaderWithoutTime(h *Header, keepSeal bool) *Header {
	newHeader := CopyWithoutTimeHeader(h)
	payload := MagicExtra
	newHeader.Extra = payload
	return newHeader
}
