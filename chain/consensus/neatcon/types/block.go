package types

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	. "github.com/nio-net/common"
	"github.com/nio-net/crypto"
	"github.com/nio-net/merkle"
	"github.com/nio-net/nio/chain/core/state"
	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/utilities/rlp"
	"github.com/nio-net/wire"
)

const MaxBlockSize = 22020096

type IntermediateBlockResult struct {
	Block    *types.Block
	State    *state.StateDB
	Receipts types.Receipts
	Ops      *types.PendingOps
}

type NCBlock struct {
	Block              *types.Block             `json:"block"`
	NTCExtra           *NeatConExtra            `json:"ntcexdata"`
	TX3ProofData       []*types.TX3ProofData    `json:"tx3proofdata"`
	IntermediateResult *IntermediateBlockResult `json:"-"`
}

func MakeBlock(height uint64, chainID string, commit *Commit,
	block *types.Block, valHash []byte, epochNumber uint64, epochBytes []byte, tx3ProofData []*types.TX3ProofData, partSize int) (*NCBlock, *PartSet) {
	NTCExtra := &NeatConExtra{
		ChainID:        chainID,
		Height:         uint64(height),
		Time:           time.Now(),
		EpochNumber:    epochNumber,
		ValidatorsHash: valHash,
		SeenCommit:     commit,
		EpochBytes:     epochBytes,
	}

	ncBlock := &NCBlock{
		Block:        block,
		NTCExtra:     NTCExtra,
		TX3ProofData: tx3ProofData,
	}
	return ncBlock, ncBlock.MakePartSet(partSize)
}

func (b *NCBlock) ValidateBasic(ncExtra *NeatConExtra) error {

	if b.NTCExtra.ChainID != ncExtra.ChainID {
		return errors.New(Fmt("Wrong Block.Header.ChainID. Expected %v, got %v", ncExtra.ChainID, b.NTCExtra.ChainID))
	}
	if b.NTCExtra.Height != ncExtra.Height+1 {
		return errors.New(Fmt("Wrong Block.Header.Height. Expected %v, got %v", ncExtra.Height+1, b.NTCExtra.Height))
	}

	return nil
}

func (b *NCBlock) FillSeenCommitHash() {
	if b.NTCExtra.SeenCommitHash == nil {
		b.NTCExtra.SeenCommitHash = b.NTCExtra.SeenCommit.Hash()
	}
}

func (b *NCBlock) Hash() []byte {
	if b == nil || b.NTCExtra.SeenCommit == nil {
		return nil
	}
	b.FillSeenCommitHash()
	return b.NTCExtra.Hash()
}

func (b *NCBlock) MakePartSet(partSize int) *PartSet {

	return NewPartSetFromData(b.ToBytes(), partSize)
}

func (b *NCBlock) ToBytes() []byte {

	type TmpBlock struct {
		BlockData    []byte
		NTCExtra     *NeatConExtra
		TX3ProofData []*types.TX3ProofData
	}

	bs, err := rlp.EncodeToBytes(b.Block)
	if err != nil {
		log.Warnf("NCBlock.toBytes error\n")
	}
	bb := &TmpBlock{
		BlockData:    bs,
		NTCExtra:     b.NTCExtra,
		TX3ProofData: b.TX3ProofData,
	}

	ret := wire.BinaryBytes(bb)
	return ret
}

func (b *NCBlock) FromBytes(reader io.Reader) (*NCBlock, error) {

	type TmpBlock struct {
		BlockData    []byte
		NTCExtra     *NeatConExtra
		TX3ProofData []*types.TX3ProofData
	}

	var n int
	var err error
	bb := wire.ReadBinary(&TmpBlock{}, reader, MaxBlockSize, &n, &err).(*TmpBlock)
	if err != nil {
		log.Warnf("NCBlock.FromBytes 0 error: %v\n", err)
		return nil, err
	}

	var block types.Block
	err = rlp.DecodeBytes(bb.BlockData, &block)
	if err != nil {
		log.Warnf("NCBlock.FromBytes 1 error: %v\n", err)
		return nil, err
	}

	ncBlock := &NCBlock{
		Block:        &block,
		NTCExtra:     bb.NTCExtra,
		TX3ProofData: bb.TX3ProofData,
	}

	log.Debugf("NCBlock.FromBytes 2 with: %v\n", ncBlock)
	return ncBlock, nil
}

func (b *NCBlock) HashesTo(hash []byte) bool {
	if len(hash) == 0 {
		return false
	}
	if b == nil {
		return false
	}
	return bytes.Equal(b.Hash(), hash)
}

func (b *NCBlock) String() string {
	return b.StringIndented("")
}

func (b *NCBlock) StringIndented(indent string) string {
	if b == nil {
		return "nil-Block"
	}

	return fmt.Sprintf(`Block{
%s  %v
%s  %v
%s  %v
%s}#%X`,
		indent, b.Block.String(),
		indent, b.NTCExtra,
		indent, b.NTCExtra.SeenCommit.StringIndented(indent+""),
		indent, b.Hash())
}

func (b *NCBlock) StringShort() string {
	if b == nil {
		return "nil-Block"
	} else {
		return fmt.Sprintf("Block#%X", b.Hash())
	}
}

type Commit struct {
	BlockID BlockID `json:"blockID"`
	Height  uint64  `json:"height"`
	Round   int     `json:"round"`

	SignAggr crypto.BLSSignature `json:"SignAggr"`
	BitArray *BitArray

	hash []byte
}

func (commit *Commit) Type() byte {
	return VoteTypePrecommit
}

func (commit *Commit) Size() int {
	return (int)(commit.BitArray.Size())
}

func (commit *Commit) NumCommits() int {
	return (int)(commit.BitArray.NumBitsSet())
}

func (commit *Commit) ValidateBasic() error {
	if commit.BlockID.IsZero() {
		return errors.New("Commit cannot be for nil block")
	}

	return nil
}

func (commit *Commit) Hash() []byte {
	if commit.hash == nil {
		hash := merkle.SimpleHashFromBinary(*commit)
		commit.hash = hash
	}
	return commit.hash
}

func (commit *Commit) StringIndented(indent string) string {
	if commit == nil {
		return ""
	}
	return ""
}

type BlockID struct {
	Hash        []byte        `json:"hash"`
	PartsHeader PartSetHeader `json:"parts"`
}

func (blockID BlockID) IsZero() bool {
	return len(blockID.Hash) == 0 && blockID.PartsHeader.IsZero()
}

func (blockID BlockID) Equals(other BlockID) bool {
	return bytes.Equal(blockID.Hash, other.Hash) &&
		blockID.PartsHeader.Equals(other.PartsHeader)
}

func (blockID BlockID) Key() string {
	return string(blockID.Hash) + string(wire.BinaryBytes(blockID.PartsHeader))
}

func (blockID BlockID) WriteSignBytes(w io.Writer, n *int, err *error) {
	if blockID.IsZero() {
		wire.WriteTo([]byte("null"), w, n, err)
	} else {
		wire.WriteJSON(CanonicalBlockID(blockID), w, n, err)
	}

}

func (blockID BlockID) String() string {
	return fmt.Sprintf(`%X:%v`, blockID.Hash, blockID.PartsHeader)
}
