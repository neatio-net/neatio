package types

import (
	"errors"
	"math/big"

	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/hexutil"
)

var (
	// NeatConDigest represents a hash of "NeatCon practical byzantine fault tolerance"
	// to identify whether the block is from NeatCon consensus engine
	NeatConDigest = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	NeatConExtraVanity = 32 // Fixed number of extra-data bytes reserved for validator vanity
	NeatConExtraSeal   = 65 // Fixed number of extra-data bytes reserved for validator seal

	NeatConDefaultDifficulty = big.NewInt(1)
	NeatConNilUncleHash      = CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	NeatConEmptyNonce        = BlockNonce{}
	NeatConNonce             = hexutil.MustDecode("0x88ff88ff88ff88ff") // Magic nonce number to vote on adding a new validator

	MagicExtra = []byte("neatchain_tmp_extra")

	// ErrInvalidNeatConHeaderExtra is returned if the length of extra-data is less than 32 bytes
	ErrInvalidNeatConHeaderExtra = errors.New("invalid neatcon header extra-data")
)

// NeatConFilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the NeatCon hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func NeatConFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	payload := MagicExtra
	newHeader.Extra = payload
	return newHeader
}
