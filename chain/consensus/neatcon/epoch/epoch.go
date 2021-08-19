package epoch

import (
	"errors"
	"fmt"

	ncTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/core/state"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/utilities/common"
	goCrypto "github.com/neatlib/crypto-go"
	dbm "github.com/neatlib/db-go"
	"github.com/neatlib/wire-go"

	"math/big"
	"sort"
	"strconv"
	"sync"
	"time"
)

var NextEpochNotExist = errors.New("next epoch parameters do not exist, fatal error")
var NextEpochNotEXPECTED = errors.New("next epoch parameters are not excepted, fatal error")

var BannedEpoch = big.NewInt(2)

const (
	EPOCH_NOT_EXIST = iota
	EPOCH_PROPOSED_NOT_VOTED
	EPOCH_VOTED_NOT_SAVED
	EPOCH_SAVED

	MinimumValidatorsSize = 1
	MaximumValidatorsSize = 50

	epochKey       = "Epoch:%v"
	latestEpochKey = "LatestEpoch"

	TimeForBanned  = 2 * time.Hour
	BannedDuration = 24 * time.Hour
)

type Epoch struct {
	mtx sync.Mutex
	db  dbm.DB

	Number         uint64
	RewardPerBlock *big.Int
	StartBlock     uint64
	EndBlock       uint64
	StartTime      time.Time
	EndTime        time.Time
	BlockGenerated int
	Status         int
	Validators     *ncTypes.ValidatorSet

	validatorVoteSet *EpochValidatorVoteSet
	rs               *RewardScheme
	previousEpoch    *Epoch
	nextEpoch        *Epoch

	logger log.Logger
}

func calcEpochKeyWithHeight(number uint64) []byte {
	return []byte(fmt.Sprintf(epochKey, number))
}

func InitEpoch(db dbm.DB, genDoc *ncTypes.GenesisDoc, logger log.Logger) *Epoch {

	epochNumber := db.Get([]byte(latestEpochKey))
	if epochNumber == nil {

		rewardScheme := MakeRewardScheme(db, &genDoc.RewardScheme)
		rewardScheme.Save()

		ep := MakeOneEpoch(db, &genDoc.CurrentEpoch, logger)
		ep.Save()

		ep.SetRewardScheme(rewardScheme)
		return ep
	} else {

		epNo, _ := strconv.ParseUint(string(epochNumber), 10, 64)
		return LoadOneEpoch(db, epNo, logger)
	}
}

func LoadOneEpoch(db dbm.DB, epochNumber uint64, logger log.Logger) *Epoch {

	epoch := loadOneEpoch(db, epochNumber, logger)

	rewardscheme := LoadRewardScheme(db)
	epoch.rs = rewardscheme

	epoch.validatorVoteSet = LoadEpochVoteSet(db, epochNumber)

	if epochNumber > 0 {
		epoch.previousEpoch = loadOneEpoch(db, epochNumber-1, logger)
		if epoch.previousEpoch != nil {
			epoch.previousEpoch.rs = rewardscheme
		}
	}

	epoch.nextEpoch = loadOneEpoch(db, epochNumber+1, logger)
	if epoch.nextEpoch != nil {
		epoch.nextEpoch.rs = rewardscheme

		epoch.nextEpoch.validatorVoteSet = LoadEpochVoteSet(db, epochNumber+1)
	}

	return epoch
}

func loadOneEpoch(db dbm.DB, epochNumber uint64, logger log.Logger) *Epoch {

	buf := db.Get(calcEpochKeyWithHeight(epochNumber))
	ep := FromBytes(buf)
	if ep != nil {
		ep.db = db
		ep.logger = logger
	}
	return ep
}

func MakeOneEpoch(db dbm.DB, oneEpoch *ncTypes.OneEpochDoc, logger log.Logger) *Epoch {

	fmt.Printf("MakeOneEpoch onEpoch=%v\n", oneEpoch)
	fmt.Printf("MakeOneEpoch validators=%v\n", oneEpoch.Validators)
	validators := make([]*ncTypes.Validator, len(oneEpoch.Validators))
	for i, val := range oneEpoch.Validators {

		validators[i] = &ncTypes.Validator{
			Address:        val.EthAccount.Bytes(),
			PubKey:         val.PubKey,
			VotingPower:    val.Amount,
			RemainingEpoch: val.RemainingEpoch,
		}
	}

	te := &Epoch{
		db: db,

		Number:         oneEpoch.Number,
		RewardPerBlock: oneEpoch.RewardPerBlock,
		StartBlock:     oneEpoch.StartBlock,
		EndBlock:       oneEpoch.EndBlock,
		StartTime:      time.Now(),
		EndTime:        time.Unix(0, 0),
		Status:         oneEpoch.Status,
		Validators:     ncTypes.NewValidatorSet(validators),

		logger: logger,
	}

	return te
}

func (epoch *Epoch) GetDB() dbm.DB {
	return epoch.db
}

func (epoch *Epoch) GetEpochValidatorVoteSet() *EpochValidatorVoteSet {

	if epoch.validatorVoteSet == nil {
		epoch.validatorVoteSet = LoadEpochVoteSet(epoch.db, epoch.Number)
	}
	return epoch.validatorVoteSet
}

func (epoch *Epoch) GetRewardScheme() *RewardScheme {
	return epoch.rs
}

func (epoch *Epoch) SetRewardScheme(rs *RewardScheme) {
	epoch.rs = rs
}

func (epoch *Epoch) Save() {
	epoch.mtx.Lock()
	defer epoch.mtx.Unlock()
	fmt.Printf("(epoch *Epoch) Save(), (EPOCH, ts.Bytes()) are: (%s,%v\n", calcEpochKeyWithHeight(epoch.Number), epoch.Bytes())
	epoch.db.SetSync(calcEpochKeyWithHeight(epoch.Number), epoch.Bytes())
	epoch.db.SetSync([]byte(latestEpochKey), []byte(strconv.FormatUint(epoch.Number, 10)))

	if epoch.nextEpoch != nil && epoch.nextEpoch.Status == EPOCH_VOTED_NOT_SAVED {
		epoch.nextEpoch.Status = EPOCH_SAVED

		epoch.db.SetSync(calcEpochKeyWithHeight(epoch.nextEpoch.Number), epoch.nextEpoch.Bytes())
	}

	if epoch.nextEpoch != nil && epoch.nextEpoch.validatorVoteSet != nil {

		SaveEpochVoteSet(epoch.db, epoch.nextEpoch.Number, epoch.nextEpoch.validatorVoteSet)
	}
}

func FromBytes(buf []byte) *Epoch {

	if len(buf) == 0 {
		return nil
	} else {
		ep := &Epoch{}
		err := wire.ReadBinaryBytes(buf, ep)
		if err != nil {
			log.Errorf("Load Epoch from Bytes Failed, error: %v", err)
			return nil
		}
		return ep
	}
}

func (epoch *Epoch) Bytes() []byte {
	return wire.BinaryBytes(*epoch)
}

func (epoch *Epoch) ValidateNextEpoch(next *Epoch, lastHeight uint64, lastBlockTime time.Time) error {

	myNextEpoch := epoch.ProposeNextEpoch(lastHeight, lastBlockTime)

	if !myNextEpoch.Equals(next, false) {
		log.Warnf("next epoch parameters are not expected, epoch propose next epoch: %v, next %v", myNextEpoch.String(), next.String())
		return NextEpochNotEXPECTED
	}

	return nil
}

func (epoch *Epoch) ShouldProposeNextEpoch(curBlockHeight uint64) bool {

	fmt.Printf("\n")
	fmt.Printf("✨✨✨✨✨✨✨   NEAT   ✨✨✨✨✨✨✨\n")
	fmt.Printf("Next epoch proposed: %v", epoch.nextEpoch)

	if epoch.nextEpoch != nil {

		return false
	}

	shouldPropose := curBlockHeight > epoch.StartBlock && curBlockHeight != 1 && curBlockHeight != epoch.EndBlock
	return shouldPropose
}

func (epoch *Epoch) ProposeNextEpoch(lastBlockHeight uint64, lastBlockTime time.Time) *Epoch {

	if epoch != nil {

		rewardPerBlock, blocks := epoch.estimateForNextEpoch(lastBlockHeight, lastBlockTime)

		next := &Epoch{
			mtx: epoch.mtx,
			db:  epoch.db,

			Number:         epoch.Number + 1,
			RewardPerBlock: rewardPerBlock,
			StartBlock:     epoch.EndBlock + 1,
			EndBlock:       epoch.EndBlock + blocks,
			BlockGenerated: 0,
			Status:         EPOCH_PROPOSED_NOT_VOTED,
			Validators:     epoch.Validators.Copy(),

			logger: epoch.logger,
		}

		return next
	}
	return nil
}

func (epoch *Epoch) GetNextEpoch() *Epoch {
	if epoch.nextEpoch == nil {
		epoch.nextEpoch = loadOneEpoch(epoch.db, epoch.Number+1, epoch.logger)
		if epoch.nextEpoch != nil {
			epoch.nextEpoch.rs = epoch.rs
			epoch.nextEpoch.validatorVoteSet = LoadEpochVoteSet(epoch.db, epoch.Number+1)
		}
	}
	return epoch.nextEpoch
}

func (epoch *Epoch) SetNextEpoch(next *Epoch) {
	if next != nil {
		next.db = epoch.db
		next.rs = epoch.rs
		next.logger = epoch.logger
	}
	epoch.nextEpoch = next
}

func (epoch *Epoch) GetPreviousEpoch() *Epoch {
	return epoch.previousEpoch
}

func (epoch *Epoch) ShouldEnterNewEpoch(height uint64, state *state.StateDB) (bool, *ncTypes.ValidatorSet, error) {

	if height == epoch.EndBlock {
		epoch.nextEpoch = epoch.GetNextEpoch()
		if epoch.nextEpoch != nil {

			for refundAddress := range state.GetDelegateAddressRefundSet() {
				state.ForEachProxied(refundAddress, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
					if pendingRefundBalance.Sign() > 0 {

						state.SubDepositProxiedBalanceByUser(refundAddress, key, pendingRefundBalance)
						state.SubPendingRefundBalanceByUser(refundAddress, key, pendingRefundBalance)
						state.SubDelegateBalance(key, pendingRefundBalance)
						state.AddBalance(key, pendingRefundBalance)
					}
					return true
				})

				if !state.IsCandidate(refundAddress) {
					state.ClearCommission(refundAddress)
				}
			}
			state.ClearDelegateRefundSet()

			var (
				refunds []*ncTypes.RefundValidatorAmount
			)

			newValidators := epoch.Validators.Copy()

			nextEpochVoteSet := epoch.GetNextEpoch().GetEpochValidatorVoteSet().Copy()
			candidateList := state.GetCandidateSet()

			for _, v := range newValidators.Validators {
				vAddr := common.BytesToAddress(v.Address)
				if !state.GetBanned(vAddr) {
					fmt.Printf("Should enter new epoch, validator %v is not banned\n", vAddr.String())
					totalProxiedBalance := new(big.Int).Add(state.GetTotalProxiedBalance(vAddr), state.GetTotalDepositProxiedBalance(vAddr))

					newVotingPower := new(big.Int).Add(totalProxiedBalance, state.GetDepositBalance(vAddr))
					if newVotingPower.Sign() == 0 {
						newValidators.Remove(v.Address)
					} else {
						v.VotingPower = newVotingPower
					}
				} else {
					newValidators.Remove(v.Address)
					delete(candidateList, vAddr)

					bannedEpoch := state.GetBannedTime(vAddr)
					if bannedEpoch.Cmp(common.Big0) == 1 {
						bannedEpoch.Sub(bannedEpoch, common.Big1)
						state.SetBannedTime(vAddr, bannedEpoch)
						fmt.Printf("Should enter new epoch 1, left banned epoch is %v\n", bannedEpoch)
					}

					refunds = append(refunds, &ncTypes.RefundValidatorAmount{Address: vAddr, Amount: v.VotingPower, Voteout: true})
					epoch.logger.Debugf("Should enter new epoch, validator %v is banned", vAddr.String())
				}
			}

			if nextEpochVoteSet == nil {
				nextEpochVoteSet = NewEpochValidatorVoteSet()
				fmt.Printf("Should enter new epoch, next epoch vote set is nil, %v\n", nextEpochVoteSet)
			}

			if len(candidateList) > 0 {
				for addr := range candidateList {
					if state.GetBanned(addr) {
						delete(candidateList, addr)

						bannedEpoch := state.GetBannedTime(addr)
						if bannedEpoch.Cmp(common.Big0) == 1 {
							bannedEpoch.Sub(bannedEpoch, common.Big1)
							state.SetBannedTime(addr, bannedEpoch)
							fmt.Printf("Should enter new epoch 2, left banned epoch is %v\n", bannedEpoch)
						}
					}
				}

				epoch.logger.Debugf("Add candidate to next epoch vote set before, candidate: %v", candidateList)

				for _, v := range newValidators.Validators {
					vAddr := common.BytesToAddress(v.Address)
					delete(candidateList, vAddr)
				}

				for _, v := range nextEpochVoteSet.Votes {
					delete(candidateList, v.Address)
				}

				epoch.logger.Debugf("Add candidate to next epoch vote set after, candidate: %v", candidateList)

				var voteArr []*EpochValidatorVote
				for addr := range candidateList {
					if state.IsCandidate(addr) {
						proxiedBalance := state.GetTotalProxiedBalance(addr)
						depositProxiedBalance := state.GetTotalDepositProxiedBalance(addr)
						pendingRefundBalance := state.GetTotalPendingRefundBalance(addr)
						netProxied := new(big.Int).Sub(new(big.Int).Add(proxiedBalance, depositProxiedBalance), pendingRefundBalance)

						if netProxied.Sign() == -1 {
							continue
						}

						state.ForEachProxied(addr, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
							state.SubProxiedBalanceByUser(addr, key, proxiedBalance)
							state.AddDepositProxiedBalanceByUser(addr, key, proxiedBalance)
							return true
						})

						pubkey := state.GetPubkey(addr)
						pubkeyBytes := common.FromHex(pubkey)
						if pubkey == "" || len(pubkeyBytes) != 128 {
							continue
						}
						var blsPK goCrypto.BLSPubKey
						copy(blsPK[:], pubkeyBytes)

						vote := &EpochValidatorVote{
							Address: addr,
							Amount:  netProxied,
							PubKey:  blsPK,
							Salt:    "neatio",
							TxHash:  common.Hash{},
						}
						voteArr = append(voteArr, vote)
						fmt.Printf("vote %v\n", vote)
					}
				}

				sort.Slice(voteArr, func(i, j int) bool {
					if voteArr[i].Amount.Cmp(voteArr[j].Amount) == 0 {
						return compareAddress(voteArr[i].Address[:], voteArr[j].Address[:])
					} else {
						return voteArr[i].Amount.Cmp(voteArr[j].Amount) == 1
					}
				})

				for i := range voteArr {
					fmt.Printf("address:%v, amount: %v\n", voteArr[i].Address.String(), voteArr[i].Amount)
					nextEpochVoteSet.StoreVote(voteArr[i])
				}
			}

			refundsUpdate, err := updateEpochValidatorSet(newValidators, nextEpochVoteSet)
			if err != nil {
				epoch.logger.Warn("Error changing validator set", "error", err)
				return false, nil, err
			}
			refunds = append(refunds, refundsUpdate...)

			for _, v := range newValidators.Validators {
				vAddr := common.BytesToAddress(v.Address)
				if state.IsCandidate(vAddr) && state.GetTotalProxiedBalance(vAddr).Sign() > 0 {
					state.ForEachProxied(vAddr, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
						if proxiedBalance.Sign() > 0 {
							state.SubProxiedBalanceByUser(vAddr, key, proxiedBalance)
							state.AddDepositProxiedBalanceByUser(vAddr, key, proxiedBalance)
						}
						return true
					})
				}
			}

			for _, r := range refunds {
				if !r.Voteout {
					state.SubDepositBalance(r.Address, r.Amount)
					state.AddBalance(r.Address, r.Amount)
				} else {
					if state.IsCandidate(r.Address) {
						state.ForEachProxied(r.Address, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
							if depositProxiedBalance.Sign() > 0 {
								state.SubDepositProxiedBalanceByUser(r.Address, key, depositProxiedBalance)
								state.AddProxiedBalanceByUser(r.Address, key, depositProxiedBalance)
							}
							return true
						})
					}
					depositBalance := state.GetDepositBalance(r.Address)
					state.SubDepositBalance(r.Address, depositBalance)
					state.AddBalance(r.Address, depositBalance)
				}
			}

			return true, newValidators, nil
		} else {
			return false, nil, NextEpochNotExist
		}
	}
	return false, nil, nil
}

func compareAddress(addrA, addrB []byte) bool {
	if addrA[0] == addrB[0] {
		return compareAddress(addrA[1:], addrB[1:])
	} else {
		return addrA[0] > addrB[0]
	}
}

func (epoch *Epoch) EnterNewEpoch(newValidators *ncTypes.ValidatorSet) (*Epoch, error) {
	if epoch.nextEpoch != nil {
		now := time.Now()

		epoch.EndTime = now
		epoch.Save()
		epoch.logger.Infof("Epoch %v reach to his end", epoch.Number)

		nextEpoch := epoch.nextEpoch
		nextEpoch.previousEpoch = epoch.Copy()
		nextEpoch.StartTime = now
		nextEpoch.Validators = newValidators

		nextEpoch.nextEpoch = nil
		nextEpoch.Save()
		epoch.logger.Infof("Enter into New Epoch %v", nextEpoch)
		return nextEpoch, nil
	} else {
		return nil, NextEpochNotExist
	}
}

func DryRunUpdateEpochValidatorSet(state *state.StateDB, validators *ncTypes.ValidatorSet, voteSet *EpochValidatorVoteSet) error {

	for _, v := range validators.Validators {
		vAddr := common.BytesToAddress(v.Address)

		totalProxiedBalance := new(big.Int).Add(state.GetTotalProxiedBalance(vAddr), state.GetTotalDepositProxiedBalance(vAddr))
		totalProxiedBalance.Sub(totalProxiedBalance, state.GetTotalPendingRefundBalance(vAddr))

		newVotingPower := new(big.Int).Add(totalProxiedBalance, state.GetDepositBalance(vAddr))
		if newVotingPower.Sign() == 0 {
			validators.Remove(v.Address)
		} else {
			v.VotingPower = newVotingPower
		}
	}

	_, err := updateEpochValidatorSet(validators, voteSet)
	return err
}

func updateEpochValidatorSet(validators *ncTypes.ValidatorSet, voteSet *EpochValidatorVoteSet) ([]*ncTypes.RefundValidatorAmount, error) {

	var refund []*ncTypes.RefundValidatorAmount
	oldValSize, newValSize := validators.Size(), 0

	if !voteSet.IsEmpty() {
		for _, v := range voteSet.Votes {
			if v.Amount == nil || v.Salt == "" || v.PubKey == nil {
				continue
			}

			_, validator := validators.GetByAddress(v.Address[:])
			if validator == nil {
				added := validators.Add(ncTypes.NewValidator(v.Address[:], v.PubKey, v.Amount))
				if !added {
					return nil, fmt.Errorf("Failed to add new validator %v with voting power %d", v.Address, v.Amount)
				}
				newValSize++
			} else if v.Amount.Sign() == 0 {
				refund = append(refund, &ncTypes.RefundValidatorAmount{Address: v.Address, Amount: validator.VotingPower, Voteout: false})
				_, removed := validators.Remove(validator.Address)
				if !removed {
					return nil, fmt.Errorf("Failed to remove validator %v", validator.Address)
				}
			} else {
				if v.Amount.Cmp(validator.VotingPower) == -1 {
					refundAmount := new(big.Int).Sub(validator.VotingPower, v.Amount)
					refund = append(refund, &ncTypes.RefundValidatorAmount{Address: v.Address, Amount: refundAmount, Voteout: false})
				}

				validator.VotingPower = v.Amount
				updated := validators.Update(validator)
				if !updated {
					return nil, fmt.Errorf("Failed to update validator %v with voting power %d", validator.Address, v.Amount)
				}
			}
		}
	}

	valSize := oldValSize + newValSize
	if valSize > MaximumValidatorsSize {
		valSize = MaximumValidatorsSize
	} else if valSize < MinimumValidatorsSize {
		valSize = MinimumValidatorsSize
	}

	for _, v := range validators.Validators {
		if v.RemainingEpoch > 0 {
			v.RemainingEpoch--
		}
	}

	if validators.Size() > valSize {
		sort.Slice(validators.Validators, func(i, j int) bool {
			if validators.Validators[i].RemainingEpoch == validators.Validators[j].RemainingEpoch {
				return validators.Validators[i].VotingPower.Cmp(validators.Validators[j].VotingPower) == 1
			} else {
				return validators.Validators[i].RemainingEpoch > validators.Validators[j].RemainingEpoch
			}
		})
		knockout := validators.Validators[valSize:]
		for _, k := range knockout {
			refund = append(refund, &ncTypes.RefundValidatorAmount{Address: common.BytesToAddress(k.Address), Amount: nil, Voteout: true})
		}

		validators.Validators = validators.Validators[:valSize]
	}

	return refund, nil
}

func (epoch *Epoch) GetEpochByBlockNumber(blockNumber uint64) *Epoch {

	if blockNumber >= epoch.StartBlock && blockNumber <= epoch.EndBlock {
		return epoch
	}

	for number := epoch.Number - 1; number >= 0; number-- {

		ep := loadOneEpoch(epoch.db, number, epoch.logger)
		if ep == nil {
			return nil
		}

		if blockNumber >= ep.StartBlock && blockNumber <= ep.EndBlock {
			return ep
		}
	}

	return nil
}

func (epoch *Epoch) Copy() *Epoch {
	return epoch.copy(true)
}

func (epoch *Epoch) copy(copyPrevNext bool) *Epoch {

	var previousEpoch, nextEpoch *Epoch
	if copyPrevNext {
		if epoch.previousEpoch != nil {
			previousEpoch = epoch.previousEpoch.copy(false)
		}

		if epoch.nextEpoch != nil {
			nextEpoch = epoch.nextEpoch.copy(false)
		}
	}

	return &Epoch{
		mtx:    epoch.mtx,
		db:     epoch.db,
		logger: epoch.logger,

		rs: epoch.rs,

		Number:           epoch.Number,
		RewardPerBlock:   new(big.Int).Set(epoch.RewardPerBlock),
		StartBlock:       epoch.StartBlock,
		EndBlock:         epoch.EndBlock,
		StartTime:        epoch.StartTime,
		EndTime:          epoch.EndTime,
		BlockGenerated:   epoch.BlockGenerated,
		Status:           epoch.Status,
		Validators:       epoch.Validators.Copy(),
		validatorVoteSet: epoch.validatorVoteSet.Copy(),

		previousEpoch: previousEpoch,
		nextEpoch:     nextEpoch,
	}
}

func (epoch *Epoch) estimateForNextEpoch(lastBlockHeight uint64, lastBlockTime time.Time) (rewardPerBlock *big.Int, blocksOfNextEpoch uint64) {

	var rewardFirstYear = epoch.rs.RewardFirstYear
	var epochNumberPerYear = epoch.rs.EpochNumberPerYear
	var totalYear = epoch.rs.TotalYear
	var timePerBlockOfEpoch int64

	const EMERGENCY_BLOCKS_OF_NEXT_EPOCH uint64 = 100

	zeroEpoch := loadOneEpoch(epoch.db, 0, epoch.logger)
	initStartTime := zeroEpoch.StartTime

	thisYear := epoch.Number / epochNumberPerYear
	nextYear := thisYear + 1

	log.Infof("estimateForNextEpoch, previous epoch %v, current epoch %v, last block height %v, epoch start block %v", epoch.previousEpoch, epoch, lastBlockHeight, epoch.StartBlock)

	if epoch.previousEpoch != nil {
		log.Infof("estimateForNextEpoch previous epoch, start time %v, end time %v", epoch.previousEpoch.StartTime.UnixNano(), epoch.previousEpoch.EndTime.UnixNano())
		prevEpoch := epoch.previousEpoch
		timePerBlockOfEpoch = prevEpoch.EndTime.Sub(prevEpoch.StartTime).Nanoseconds() / int64(prevEpoch.EndBlock-prevEpoch.StartBlock)
	} else {
		timePerBlockOfEpoch = lastBlockTime.Sub(epoch.StartTime).Nanoseconds() / int64(lastBlockHeight-epoch.StartBlock)
	}

	if timePerBlockOfEpoch == 0 {
		timePerBlockOfEpoch = 1000000000
	}

	epochLeftThisYear := epochNumberPerYear - epoch.Number%epochNumberPerYear - 1

	blocksOfNextEpoch = 0

	log.Info("estimateForNextEpoch",
		"epochLeftThisYear", epochLeftThisYear,
		"timePerBlockOfEpoch", timePerBlockOfEpoch)

	if epochLeftThisYear == 0 {

		nextYearStartTime := initStartTime.AddDate(int(nextYear), 0, 0)

		nextYearEndTime := nextYearStartTime.AddDate(1, 0, 0)

		timeLeftNextYear := nextYearEndTime.Sub(nextYearStartTime)

		epochLeftNextYear := epochNumberPerYear

		epochTimePerEpochLeftNextYear := timeLeftNextYear.Nanoseconds() / int64(epochLeftNextYear)

		blocksOfNextEpoch = uint64(epochTimePerEpochLeftNextYear / timePerBlockOfEpoch)

		log.Info("estimateForNextEpoch 0",
			"timePerBlockOfEpoch", timePerBlockOfEpoch,
			"nextYearStartTime", nextYearStartTime,
			"timeLeftNextYear", timeLeftNextYear,
			"epochLeftNextYear", epochLeftNextYear,
			"epochTimePerEpochLeftNextYear", epochTimePerEpochLeftNextYear,
			"blocksOfNextEpoch", blocksOfNextEpoch)

		if blocksOfNextEpoch < EMERGENCY_BLOCKS_OF_NEXT_EPOCH {
			blocksOfNextEpoch = EMERGENCY_BLOCKS_OF_NEXT_EPOCH
			epoch.logger.Error("EstimateForNextEpoch Error: Please check the epoch_no_per_year setup in Genesis")
		}

		rewardPerEpochNextYear := calculateRewardPerEpochByYear(rewardFirstYear, int64(nextYear), int64(totalYear), int64(epochNumberPerYear))

		rewardPerBlock = new(big.Int).Div(rewardPerEpochNextYear, big.NewInt(int64(blocksOfNextEpoch)))

	} else {

		nextYearStartTime := initStartTime.AddDate(int(nextYear), 0, 0)

		timeLeftThisYear := nextYearStartTime.Sub(lastBlockTime)

		if timeLeftThisYear > 0 {

			epochTimePerEpochLeftThisYear := timeLeftThisYear.Nanoseconds() / int64(epochLeftThisYear)

			blocksOfNextEpoch = uint64(epochTimePerEpochLeftThisYear / timePerBlockOfEpoch)

			log.Info("estimateForNextEpoch 1",
				"timePerBlockOfEpoch", timePerBlockOfEpoch,
				"nextYearStartTime", nextYearStartTime,
				"timeLeftThisYear", timeLeftThisYear,
				"epochTimePerEpochLeftThisYear", epochTimePerEpochLeftThisYear,
				"blocksOfNextEpoch", blocksOfNextEpoch)
		}

		if blocksOfNextEpoch < EMERGENCY_BLOCKS_OF_NEXT_EPOCH {
			blocksOfNextEpoch = EMERGENCY_BLOCKS_OF_NEXT_EPOCH
			epoch.logger.Error("EstimateForNextEpoch Error: Please check the epoch_no_per_year setup in Genesis")
		}

		log.Debugf("Current Epoch Number %v, This Year %v, Next Year %v, Epoch No Per Year %v, Epoch Left This year %v\n"+
			"initStartTime %v ; nextYearStartTime %v\n"+
			"Time Left This year %v, timePerBlockOfEpoch %v, blocksOfNextEpoch %v\n", epoch.Number, thisYear, nextYear, epochNumberPerYear, epochLeftThisYear, initStartTime, nextYearStartTime, timeLeftThisYear, timePerBlockOfEpoch, blocksOfNextEpoch)

		rewardPerEpochThisYear := calculateRewardPerEpochByYear(rewardFirstYear, int64(thisYear), int64(totalYear), int64(epochNumberPerYear))

		rewardPerBlock = new(big.Int).Div(rewardPerEpochThisYear, big.NewInt(int64(blocksOfNextEpoch)))

	}
	return rewardPerBlock, blocksOfNextEpoch
}

func calculateRewardPerEpochByYear(rewardFirstYear *big.Int, year, totalYear, epochNumberPerYear int64) *big.Int {
	if year > totalYear {
		return big.NewInt(0)
	}

	return new(big.Int).Div(rewardFirstYear, big.NewInt(epochNumberPerYear))
}

func (epoch *Epoch) Equals(other *Epoch, checkPrevNext bool) bool {

	if (epoch == nil && other != nil) || (epoch != nil && other == nil) {
		return false
	}

	if epoch == nil && other == nil {
		log.Debugf("Epoch equals epoch %v, other %v", epoch, other)
		return true
	}

	if !(epoch.Number == other.Number && epoch.RewardPerBlock.Cmp(other.RewardPerBlock) == 0 &&
		epoch.StartBlock == other.StartBlock && epoch.EndBlock == other.EndBlock &&
		epoch.Validators.Equals(other.Validators)) {
		return false
	}

	if checkPrevNext {
		if !epoch.previousEpoch.Equals(other.previousEpoch, false) ||
			!epoch.nextEpoch.Equals(other.nextEpoch, false) {
			return false
		}
	}
	log.Debugf("Epoch equals end, no matching")
	return true
}

func (epoch *Epoch) String() string {

	return fmt.Sprintf(
		"Number %v,\n"+
			"NEAT Reward per block: %v,\n"+
			"Epoch starts at block: %v,\n"+
			"Epoch ending at block: %v,\n",
		epoch.Number,
		epoch.RewardPerBlock,
		epoch.StartBlock,
		epoch.EndBlock,
	)
}

func UpdateEpochEndTime(db dbm.DB, epNumber uint64, endTime time.Time) {
	ep := loadOneEpoch(db, epNumber, nil)
	if ep != nil {
		ep.mtx.Lock()
		defer ep.mtx.Unlock()
		ep.EndTime = endTime
		db.SetSync(calcEpochKeyWithHeight(epNumber), ep.Bytes())
	}
}

func (epoch *Epoch) GetBannedDuration() time.Duration {
	return BannedDuration
}

func (epoch *Epoch) UpdateBannedState(header *types.Header, prevHeader *types.Header, commit *ncTypes.Commit, state *state.StateDB) {
	validators := epoch.Validators.Validators
	height := header.Number.Uint64()

	if height <= 1 || height == epoch.StartBlock {
		return
	} else if height == epoch.EndBlock {
		for _, v := range validators {
			addr := common.BytesToAddress(v.Address[:])
			state.SetMinedBlocks(addr, common.Big0)
		}
	} else if height == (epoch.EndBlock - 1) {
		epoch.logger.Debugf("Update validator banned state, epoch end block - 1 %v", height)
		for _, v := range validators {
			addr := common.BytesToAddress(v.Address[:])
			times := state.GetMinedBlocks(addr)
			if times.Cmp(common.Big0) == 0 {
				epoch.logger.Debugf("Update validator banned state, set %v banned, mined blocks %v, banned epoch %v", addr.String(), times, BannedEpoch)
				state.SetBanned(addr, true)
				state.SetBannedTime(addr, BannedEpoch)

				state.MarkAddressBanned(addr)
			}
		}
	} else {
		if commit == nil || commit.BitArray == nil {
			epoch.logger.Debugf("Update validator banned state seenCommit %v", commit)
			return
		}

		bitMap := commit.BitArray
		for i := uint64(0); i < bitMap.Size(); i++ {
			if bitMap.GetIndex(i) {
				addr := common.BytesToAddress(validators[i].Address)

				times := state.GetMinedBlocks(addr)
				newTimes := big.NewInt(0)
				newTimes.Add(times, common.Big1)

				state.SetMinedBlocks(addr, newTimes)

				epoch.logger.Debugf("Update validator banned state, %v new mined block times %v, current times %v", addr.String(), newTimes, times)
			}
		}
	}

}
