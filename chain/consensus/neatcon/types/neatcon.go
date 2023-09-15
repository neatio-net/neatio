package types

import (
	"fmt"
	"time"

	"github.com/nio-net/merkle"
	neatTypes "github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/utilities/common/hexutil"
	"github.com/nio-net/wire"
)

type NeatConExtra struct {
	ChainID         string    `json:"chain_id"`
	Height          uint64    `json:"height"`
	Time            time.Time `json:"time"`
	NeedToSave      bool      `json:"need_to_save"`
	NeedToBroadcast bool      `json:"need_to_broadcast"`
	EpochNumber     uint64    `json:"epoch_number"`
	SeenCommitHash  []byte    `json:"last_commit_hash"`
	ValidatorsHash  []byte    `json:"validators_hash"`
	SeenCommit      *Commit   `json:"seen_commit"`
	EpochBytes      []byte    `json:"epoch_bytes"`
}

func (te *NeatConExtra) Copy() *NeatConExtra {

	return &NeatConExtra{
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

func (te *NeatConExtra) Hash() []byte {
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

func ExtractNeatConExtra(h *neatTypes.Header) (*NeatConExtra, error) {

	if len(h.Extra) == 0 {
		return &NeatConExtra{}, nil
	}

	var ncExtra = NeatConExtra{}
	err := wire.ReadBinaryBytes(h.Extra[:], &ncExtra)

	if err != nil {
		return nil, err
	}
	return &ncExtra, nil
}

func (te *NeatConExtra) String() string {
	str := fmt.Sprintf(`Network info: {
ChainID:   %s
EpochNo.:  %v
Height:    %v
Timestamp: %v

}
`, te.ChainID, te.EpochNumber, te.Height, te.Time)
	return str
}

func DecodeExtraData(extra string) (ncExtra *NeatConExtra, err error) {
	ncExtra = &NeatConExtra{}
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
