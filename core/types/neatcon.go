package types

import (
	"errors"
	"math/big"

	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/common/hexutil"
)

var (
	// NeatconDigest represents a hash of "NeatCon practical byzantine fault tolerance"
	// to identify whether the block is from NeatCon consensus engine
	NeatconDigest = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	NeatconExtraVanity = 32 // Fixed number of extra-data bytes reserved for validator vanity
	NeatconExtraSeal   = 65 // Fixed number of extra-data bytes reserved for validator seal

	NeatconDefaultDifficulty = big.NewInt(1)
	NeatconNilUncleHash      = CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	NeatconEmptyNonce        = BlockNonce{}
	NeatconNonce             = hexutil.MustDecode("0x88ff88ff88ff88ff") // Magic nonce number to vote on adding a new validator

	MagicExtra = []byte("neatio_tmp_extra")

	// ErrInvalidNeatconHeaderExtra is returned if the length of extra-data is less than 32 bytes
	ErrInvalidNeatconHeaderExtra = errors.New("invalid neatpos header extra-data")
)

// NeatconFilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the NeatCon hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func NeatconFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	payload := MagicExtra
	newHeader.Extra = payload
	return newHeader
}
