package neatcon

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/nio-net/crypto"
	"github.com/nio-net/nio/chain/consensus"
	"github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	ntcTypes "github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/common/hexutil"
	neatCrypto "github.com/nio-net/nio/utilities/crypto"
)

type API struct {
	chain   consensus.ChainReader
	neatcon *backend
}

func (api *API) GetCurrentEpochNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(api.neatcon.core.consensusState.Epoch.Number), nil
}

func (api *API) GetEpoch(num hexutil.Uint64) (*ntcTypes.EpochApiForConsole, error) {

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

	validators := make([]*ntcTypes.EpochValidatorForConsole, len(resultEpoch.Validators.Validators))
	for i, val := range resultEpoch.Validators.Validators {
		validators[i] = &ntcTypes.EpochValidatorForConsole{
			Address:        common.BytesToAddress(val.Address).String(),
			PubKey:         val.PubKey.KeyString(),
			Amount:         (*hexutil.Big)(val.VotingPower),
			RemainingEpoch: hexutil.Uint64(val.RemainingEpoch),
		}
	}

	return &ntcTypes.EpochApiForConsole{
		Number:         hexutil.Uint64(resultEpoch.Number),
		RewardPerBlock: (*hexutil.Big)(resultEpoch.RewardPerBlock),
		StartBlock:     hexutil.Uint64(resultEpoch.StartBlock),
		EndBlock:       hexutil.Uint64(resultEpoch.EndBlock),
		StartTime:      resultEpoch.StartTime,
		EndTime:        resultEpoch.EndTime,
		Validators:     validators,
	}, nil
}

func (api *API) GetNextEpochVote() (*ntcTypes.EpochVotesApiForConsole, error) {

	ep := api.neatcon.core.consensusState.Epoch
	if ep.GetNextEpoch() != nil {

		var votes []*epoch.EpochValidatorVote
		if ep.GetNextEpoch().GetEpochValidatorVoteSet() != nil {
			votes = ep.GetNextEpoch().GetEpochValidatorVoteSet().Votes
		}
		votesApi := make([]*ntcTypes.EpochValidatorVoteApiForConsole, 0, len(votes))
		for _, v := range votes {
			var pkstring string
			if v.PubKey != nil {
				pkstring = v.PubKey.KeyString()
			}

			votesApi = append(votesApi, &ntcTypes.EpochValidatorVoteApiForConsole{
				EpochValidatorForConsole: ntcTypes.EpochValidatorForConsole{
					Address: v.Address.String(),
					PubKey:  pkstring,
					Amount:  (*hexutil.Big)(v.Amount),
				},
				Salt:     v.Salt,
				VoteHash: v.VoteHash,
				TxHash:   v.TxHash,
			})
		}

		return &ntcTypes.EpochVotesApiForConsole{
			EpochNumber: hexutil.Uint64(ep.GetNextEpoch().Number),
			StartBlock:  hexutil.Uint64(ep.GetNextEpoch().StartBlock),
			EndBlock:    hexutil.Uint64(ep.GetNextEpoch().EndBlock),
			Votes:       votesApi,
		}, nil
	}
	return nil, errors.New("next epoch has not been proposed")
}

func (api *API) GetNextEpochValidators() ([]*ntcTypes.EpochValidatorForConsole, error) {

	//height := api.chain.CurrentBlock().NumberU64()

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

		validators := make([]*ntcTypes.EpochValidatorForConsole, 0, len(nextValidators.Validators))
		for _, val := range nextValidators.Validators {
			var pkstring string
			if val.PubKey != nil {
				pkstring = val.PubKey.KeyString()
			}
			validators = append(validators, &ntcTypes.EpochValidatorForConsole{
				Address:        common.BytesToAddress(val.Address).String(),
				PubKey:         pkstring,
				Amount:         (*hexutil.Big)(val.VotingPower),
				RemainingEpoch: hexutil.Uint64(val.RemainingEpoch),
			})
		}

		return validators, nil
	}
}

// CreateValidator
func (api *API) CreateValidator(from common.Address) (*ntcTypes.PrivV, error) {
	validator := ntcTypes.GenPrivValidatorKey(from)
	privV := &ntcTypes.PrivV{
		Address: validator.Address.String(),
		PubKey:  validator.PubKey,
		PrivKey: validator.PrivKey,
	}
	return privV, nil
}

// decode extra data
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

// get consensus publickey of the block
func (api *API) GetConsensusPublicKey(extra string) ([]string, error) {
	ncExtra, err := ntcTypes.DecodeExtraData(extra)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("GetConsensusPublicKey ncExtra %v\n", ncExtra)
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

	//fmt.Printf("GetConsensusPublicKey result epoch %v\n", resultEpoch)
	validatorSet := resultEpoch.Validators
	//fmt.Printf("GetConsensusPublicKey validatorset %v\n", validatorSet)

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

func (api *API) GetValidatorStatus(from common.Address) (*ntcTypes.ValidatorStatus, error) {
	state, err := api.chain.State()
	if state == nil || err != nil {
		return nil, err
	}
	status := &ntcTypes.ValidatorStatus{
		IsBanned: state.GetOrNewStateObject(from).IsBanned(),
	}

	return status, nil
}

func (api *API) GetCandidateList() (*ntcTypes.CandidateApi, error) {
	state, err := api.chain.State()

	if state == nil || err != nil {
		return nil, err
	}

	candidateList := make([]string, 0)
	candidateSet := state.GetCandidateSet()
	fmt.Printf("candidate set %v", candidateSet)
	for addr := range candidateSet {
		candidateList = append(candidateList, addr.String())
	}

	candidates := &ntcTypes.CandidateApi{
		CandidateList: candidateList,
	}

	return candidates, nil
}

func (api *API) GetBannedList() (*ntcTypes.BannedApi, error) {
	state, err := api.chain.State()

	if state == nil || err != nil {
		return nil, err
	}

	bannedList := make([]string, 0)
	bannedSet := state.GetBannedSet()
	fmt.Printf("banned set %v", bannedSet)
	for addr := range bannedSet {
		bannedList = append(bannedList, addr.String())
	}

	bannedAddresses := &ntcTypes.BannedApi{
		BannedList: bannedList,
	}

	return bannedAddresses, nil
}
