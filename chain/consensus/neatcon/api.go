package neatcon

import (
	"errors"
	"math/big"

	"github.com/neatlab/neatio/chain/consensus"
	"github.com/neatlab/neatio/chain/consensus/neatcon/epoch"
	ntcTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/common/hexutil"
	neatCrypto "github.com/neatio-network/neatio/utilities/crypto"
	"github.com/neatlib/crypto-go"
)

type API struct {
	chain   consensus.ChainReader
	neatcon *backend
}

func (api *API) GetCurrentEpochNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(api.neatcon.core.consensusState.Epoch.Number), nil
}

func (api *API) GetEpoch(num hexutil.Uint64) (*ntcTypes.EpochApi, error) {

	number := uint64(num)
	var resultEpoch *epoch.Epoch
	curEpoch := api.neatcon.core.consensusState.Epoch
	if number < 0 || number > curEpoch.Number {
		return nil, errors.New("epoch number out of range")
	}

	if number == curEpoch.Number {
		resultEpoch = curEpoch
	} else {
		resultEpoch = epoch.LoadOneEpoch(curEpoch.GetDB(), number, nil)
	}

	validators := make([]*ntcTypes.EpochValidator, len(resultEpoch.Validators.Validators))
	for i, val := range resultEpoch.Validators.Validators {
		validators[i] = &ntcTypes.EpochValidator{
			Address:        common.BytesToAddress(val.Address),
			PubKey:         val.PubKey.KeyString(),
			Amount:         (*hexutil.Big)(val.VotingPower),
			RemainingEpoch: hexutil.Uint64(val.RemainingEpoch),
		}
	}

	return &ntcTypes.EpochApi{
		Number:         hexutil.Uint64(resultEpoch.Number),
		RewardPerBlock: (*hexutil.Big)(resultEpoch.RewardPerBlock),
		StartBlock:     hexutil.Uint64(resultEpoch.StartBlock),
		EndBlock:       hexutil.Uint64(resultEpoch.EndBlock),
		StartTime:      resultEpoch.StartTime,
		EndTime:        resultEpoch.EndTime,
		Validators:     validators,
	}, nil
}

func (api *API) GetNextEpochVote() (*ntcTypes.EpochVotesApi, error) {

	ep := api.neatcon.core.consensusState.Epoch
	if ep.GetNextEpoch() != nil {

		var votes []*epoch.EpochValidatorVote
		if ep.GetNextEpoch().GetEpochValidatorVoteSet() != nil {
			votes = ep.GetNextEpoch().GetEpochValidatorVoteSet().Votes
		}
		votesApi := make([]*ntcTypes.EpochValidatorVoteApi, 0, len(votes))
		for _, v := range votes {
			var pkstring string
			if v.PubKey != nil {
				pkstring = v.PubKey.KeyString()
			}

			votesApi = append(votesApi, &ntcTypes.EpochValidatorVoteApi{
				EpochValidator: ntcTypes.EpochValidator{
					Address: v.Address,
					PubKey:  pkstring,
					Amount:  (*hexutil.Big)(v.Amount),
				},
				Salt:     v.Salt,
				VoteHash: v.VoteHash,
				TxHash:   v.TxHash,
			})
		}

		return &ntcTypes.EpochVotesApi{
			EpochNumber: hexutil.Uint64(ep.GetNextEpoch().Number),
			StartBlock:  hexutil.Uint64(ep.GetNextEpoch().StartBlock),
			EndBlock:    hexutil.Uint64(ep.GetNextEpoch().EndBlock),
			Votes:       votesApi,
		}, nil
	}
	return nil, errors.New("next epoch has not been proposed")
}

func (api *API) GetNextEpochValidators() ([]*ntcTypes.EpochValidator, error) {

	ep := api.neatcon.core.consensusState.Epoch
	nextEp := ep.GetNextEpoch()
	if nextEp == nil {
		return nil, errors.New("voting for next epoch has not started yet")
	} else {
		state, err := api.chain.State()
		if err != nil {
			return nil, err
		}

		nextValidators := ep.Validators.Copy()

		err = epoch.DryRunUpdateEpochValidatorSet(state, nextValidators, nextEp.GetEpochValidatorVoteSet())
		if err != nil {
			return nil, err
		}

		validators := make([]*ntcTypes.EpochValidator, 0, len(nextValidators.Validators))
		for _, val := range nextValidators.Validators {
			var pkstring string
			if val.PubKey != nil {
				pkstring = val.PubKey.KeyString()
			}
			validators = append(validators, &ntcTypes.EpochValidator{
				Address:        common.BytesToAddress(val.Address),
				PubKey:         pkstring,
				Amount:         (*hexutil.Big)(val.VotingPower),
				RemainingEpoch: hexutil.Uint64(val.RemainingEpoch),
			})
		}

		return validators, nil
	}
}

func (api *API) DecodeExtraData(extra string) (extraApi *ntcTypes.NeatConExtraApi, err error) {
	ncExtra, err := ntcTypes.DecodeExtraData(extra)
	if err != nil {
		return nil, err
	}
	extraApi = &ntcTypes.NeatConExtraApi{
		ChainID:         ncExtra.ChainID,
		Height:          hexutil.Uint64(ncExtra.Height),
		Time:            ncExtra.Time,
		NeedToSave:      ncExtra.NeedToSave,
		NeedToBroadcast: ncExtra.NeedToBroadcast,
		EpochNumber:     hexutil.Uint64(ncExtra.EpochNumber),
		SeenCommitHash:  hexutil.Encode(ncExtra.SeenCommitHash),
		ValidatorsHash:  hexutil.Encode(ncExtra.ValidatorsHash),
		SeenCommit: &ntcTypes.CommitApi{
			BlockID: ntcTypes.BlockIDApi{
				Hash: hexutil.Encode(ncExtra.SeenCommit.BlockID.Hash),
				PartsHeader: ntcTypes.PartSetHeaderApi{
					Total: hexutil.Uint64(ncExtra.SeenCommit.BlockID.PartsHeader.Total),
					Hash:  hexutil.Encode(ncExtra.SeenCommit.BlockID.PartsHeader.Hash),
				},
			},
			Height:   hexutil.Uint64(ncExtra.SeenCommit.Height),
			Round:    ncExtra.SeenCommit.Round,
			SignAggr: ncExtra.SeenCommit.SignAggr,
			BitArray: ncExtra.SeenCommit.BitArray,
		},
		EpochBytes: ncExtra.EpochBytes,
	}
	return extraApi, nil
}

func (api *API) GetConsensusPublicKey(extra string) ([]string, error) {
	ncExtra, err := ntcTypes.DecodeExtraData(extra)
	if err != nil {
		return nil, err
	}

	number := uint64(ncExtra.EpochNumber)
	var resultEpoch *epoch.Epoch
	curEpoch := api.neatcon.core.consensusState.Epoch
	if number < 0 || number > curEpoch.Number {
		return nil, errors.New("epoch number out of range")
	}

	if number == curEpoch.Number {
		resultEpoch = curEpoch
	} else {
		resultEpoch = epoch.LoadOneEpoch(curEpoch.GetDB(), number, nil)
	}

	validatorSet := resultEpoch.Validators

	aggr, err := validatorSet.GetAggrPubKeyAndAddress(ncExtra.SeenCommit.BitArray)
	if err != nil {
		return nil, err
	}

	var pubkeys []string
	if len(aggr.PublicKeys) > 0 {
		for _, v := range aggr.PublicKeys {
			if v != "" {
				pubkeys = append(pubkeys, v)
			}
		}
	}

	return pubkeys, nil
}

func (api *API) GetVoteHash(from common.Address, pubkey crypto.BLSPubKey, amount *hexutil.Big, salt string) common.Hash {
	byteData := [][]byte{
		from.Bytes(),
		pubkey.Bytes(),
		(*big.Int)(amount).Bytes(),
		[]byte(salt),
	}
	return neatCrypto.Keccak256Hash(ConcatCopyPreAllocate(byteData))
}
