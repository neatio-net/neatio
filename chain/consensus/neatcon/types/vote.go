package types

import (
	"errors"
	"fmt"
	"io"

	. "github.com/neatio-net/common-go"
	"github.com/neatio-net/crypto-go"
	"github.com/neatio-net/neatio/utilities/rlp"
	"github.com/neatio-net/wire-go"
)

var (
	ErrVoteUnexpectedStep          = errors.New("Unexpected step")
	ErrVoteInvalidValidatorIndex   = errors.New("Invalid round vote validator index")
	ErrVoteInvalidValidatorAddress = errors.New("Invalid round vote validator address")
	ErrVoteInvalidSignature        = errors.New("Invalid round vote signature")
	ErrVoteInvalidBlockHash        = errors.New("Invalid block hash")
)

type ErrVoteConflictingVotes struct {
	VoteA *Vote
	VoteB *Vote
}

func (err *ErrVoteConflictingVotes) Error() string {
	return "Conflicting votes"
}

const (
	VoteTypePrevote   = byte(0x01)
	VoteTypePrecommit = byte(0x02)
)

func IsVoteTypeValid(type_ byte) bool {
	switch type_ {
	case VoteTypePrevote:
		return true
	case VoteTypePrecommit:
		return true
	default:
		return false
	}
}

type Vote struct {
	ValidatorAddress []byte           `json:"validator_address"`
	ValidatorIndex   uint64           `json:"validator_index"`
	Height           uint64           `json:"height"`
	Round            uint64           `json:"round"`
	Type             byte             `json:"type"`
	BlockID          BlockID          `json:"block_id"`
	Signature        crypto.Signature `json:"signature"`
	SignBytes        []byte           `json:"sign_bytes"`
}

func (vote *Vote) WriteSignBytes(chainID string, w io.Writer, n *int, err *error) {
	wire.WriteJSON(CanonicalJSONOnceVote{
		chainID,
		CanonicalVote(vote),
	}, w, n, err)
}

func (vote *Vote) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		vote.ValidatorAddress,
		vote.ValidatorIndex,
		vote.Height,
		vote.Round,
		vote.Type,
		vote.BlockID,
		vote.Signature.Bytes(),
	})
}

func (vote *Vote) DecodeRLP(s *rlp.Stream) error {
	var vt struct {
		ValidatorAddress []byte
		ValidatorIndex   uint64
		Height           uint64
		Round            uint64
		Type             byte
		BlockID          BlockID
		Signature        []byte
	}

	if err := s.Decode(&vt); err != nil {
		return err
	}

	vote.ValidatorAddress = vt.ValidatorAddress
	vote.ValidatorIndex = vt.ValidatorIndex
	vote.Height = vt.Height
	vote.Round = vt.Round
	vote.Type = vt.Type
	vote.BlockID = vt.BlockID

	sig, err := crypto.SignatureFromBytes(vt.Signature)
	if err != nil {
		return err
	}
	vote.Signature = sig

	if err != nil {
		return err
	}

	return nil
}

func (vote *Vote) Copy() *Vote {
	voteCopy := *vote
	return &voteCopy
}

func (vote *Vote) String() string {
	if vote == nil {
		return "nil-Vote"
	}
	var typeString string
	switch vote.Type {
	case VoteTypePrevote:
		typeString = "Prevote"
	case VoteTypePrecommit:
		typeString = "Precommit"
	default:
		PanicSanity("Unknown vote type")
	}

	return fmt.Sprintf("Vote{%v:%X %v/%02d/%v(%v) %X %v}",
		vote.ValidatorIndex, Fingerprint(vote.ValidatorAddress),
		vote.Height, vote.Round, vote.Type, typeString,
		Fingerprint(vote.BlockID.Hash), vote.Signature)
}
