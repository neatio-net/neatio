package state

import (
	"bytes"
	"time"

	"github.com/neatlab/neatio/chain/log"

	. "github.com/neatlib/common-go"

	"github.com/neatlib/wire-go"

	"github.com/neatlab/neatio/chain/consensus/neatcon/types"

	"github.com/neatlab/neatio/chain/consensus/neatcon/epoch"
	"github.com/pkg/errors"
)

type State struct {
	NTCExtra *types.NeatConExtra

	Epoch *epoch.Epoch

	logger log.Logger
}

func NewState(logger log.Logger) *State {
	return &State{logger: logger}
}

func (s *State) Copy() *State {

	return &State{

		NTCExtra: s.NTCExtra.Copy(),
		Epoch:    s.Epoch.Copy(),

		logger: s.logger,
	}
}

func (s *State) Equals(s2 *State) bool {
	return bytes.Equal(s.Bytes(), s2.Bytes())
}

func (s *State) Bytes() []byte {
	buf, n, err := new(bytes.Buffer), new(int), new(error)
	wire.WriteBinary(s, buf, n, err)
	if *err != nil {
		PanicCrisis(*err)
	}
	return buf.Bytes()
}

func (s *State) GetValidators() (*types.ValidatorSet, *types.ValidatorSet, error) {

	if s.Epoch == nil {
		return nil, nil, errors.New("epoch does not exist")
	}

	if s.NTCExtra.EpochNumber == uint64(s.Epoch.Number) {
		return s.Epoch.Validators, s.Epoch.Validators, nil
	} else if s.NTCExtra.EpochNumber == uint64(s.Epoch.Number-1) {
		return s.Epoch.GetPreviousEpoch().Validators, s.Epoch.Validators, nil
	}

	return nil, nil, errors.New("epoch information error")
}

func MakeGenesisState(chainID string, logger log.Logger) *State {

	return &State{

		NTCExtra: &types.NeatConExtra{
			ChainID:         chainID,
			Height:          0,
			Time:            time.Now(),
			EpochNumber:     0,
			NeedToSave:      false,
			NeedToBroadcast: false,
		},

		logger: logger,
	}
}
