package types

import (
	"fmt"
	"time"

	"github.com/neatlab/neatio/common/hexutil"
	ethTypes "github.com/neatlab/neatio/core/types"
	"github.com/neatlib/merkle-go"
	"github.com/neatlib/wire-go"
)

type NeatconExtra struct {
	ChainID         string    `json:"chain_id"`
	Height          uint64    `json:"height"`
	Time            time.Time `json:"time"`
	NeedToSave      bool      `json:"need_to_save"`
	NeedToBroadcast bool      `json:"need_to_broadcast"`
	EpochNumber     uint64    `json:"epoch_number"`
	SeenCommitHash  []byte    `json:"last_commit_hash"` // commit from validators from the last block
	ValidatorsHash  []byte    `json:"validators_hash"`  // validators for the current block
	SeenCommit      *Commit   `json:"seen_commit"`
	EpochBytes      []byte    `json:"epoch_bytes"`
}

/*
// EncodeRLP serializes ist into the Ethereum RLP format.
func (te *NeatconExtra) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		te.ChainID, te.Height, te.Time, te.LastBlockID,
		te.SeenCommitHash, te.ValidatorsHash,
		te.SeenCommit,
	})
}

// DecodeRLP implements rlp.Decoder, and load the istanbul fields from a RLP stream.
func (te *NeatconExtra) DecodeRLP(s *rlp.Stream) error {
	var ncExtra NeatconExtra
	if err := s.Decode(&ncExtra); err != nil {
		return err
	}
	te.ChainID, te.Height, te.Time, te.LastBlockID,
		te.SeenCommitHash, te.ValidatorsHash,
		te.SeenCommit = ncExtra.ChainID, ncExtra.Height, ncExtra.Time, ncExtra.LastBlockID,
		ncExtra.SeenCommitHash, ncExtra.ValidatorsHash,
		ncExtra.SeenCommit
	return nil
}
*/

//be careful, here not deep copy because just reference to SeenCommit
func (te *NeatconExtra) Copy() *NeatconExtra {
	//fmt.Printf("State.Copy(), s.LastValidators are %v\n",s.LastValidators)
	//debug.PrintStack()

	return &NeatconExtra{
		ChainID:         te.ChainID,
		Height:          te.Height,
		Time:            te.Time,
		NeedToSave:      te.NeedToSave,
		NeedToBroadcast: te.NeedToBroadcast,
		EpochNumber:     te.EpochNumber,
		SeenCommitHash:  te.SeenCommitHash,
		ValidatorsHash:  te.ValidatorsHash,
		SeenCommit:      te.SeenCommit,
		EpochBytes:      te.EpochBytes,
	}
}

// NOTE: hash is nil if required fields are missing.
func (te *NeatconExtra) Hash() []byte {
	if len(te.ValidatorsHash) == 0 {
		return nil
	}
	return merkle.SimpleHashFromMap(map[string]interface{}{
		"ChainID":         te.ChainID,
		"Height":          te.Height,
		"Time":            te.Time,
		"NeedToSave":      te.NeedToSave,
		"NeedToBroadcast": te.NeedToBroadcast,
		"EpochNumber":     te.EpochNumber,
		"Validators":      te.ValidatorsHash,
		"EpochBytes":      te.EpochBytes,
	})
}

// ExtractNeatconExtra extracts all values of the NeatconExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func ExtractNeatconExtra(h *ethTypes.Header) (*NeatconExtra, error) {

	if len(h.Extra) == 0 {
		return &NeatconExtra{}, nil
	}

	var ncExtra = NeatconExtra{}
	err := wire.ReadBinaryBytes(h.Extra[:], &ncExtra)
	//err := rlp.DecodeBytes(h.Extra[:], &ncExtra)
	if err != nil {
		return nil, err
	}
	return &ncExtra, nil
}

func (te *NeatconExtra) String() string {
	str := fmt.Sprintf(`NeatconExtra: {
ChainID:     %s
EpochNumber: %v
Height:      %v
Time:        %v

EpochBytes: length %v
}
`, te.ChainID, te.EpochNumber, te.Height, te.Time, len(te.EpochBytes))
	return str
}

func DecodeExtraData(extra string) (ncExtra *NeatconExtra, err error) {
	ncExtra = &NeatconExtra{}
	extraByte, err := hexutil.Decode(extra)
	if err != nil {
		return nil, err
	}

	err = wire.ReadBinaryBytes(extraByte, ncExtra)
	if err != nil {
		return nil, err
	}
	return ncExtra, nil
}
