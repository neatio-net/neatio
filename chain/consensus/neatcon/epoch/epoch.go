package epoch

import (
	"errors"
	"fmt"

	ncTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/core/state"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/utilities/common"
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

const (
	EPOCH_NOT_EXIST = iota
	EPOCH_PROPOSED_NOT_VOTED
	EPOCH_VOTED_NOT_SAVED
	EPOCH_SAVED

	MinimumValidatorsSize = 1
	MaximumValidatorsSize = 50

	epochKey       = "Epoch:%v"
	latestEpochKey = "LatestEpoch"
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
		logger:         logger,
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
	epoch.db.SetSync(calcEpochKeyWithHeight(epoch.Number), epoch.Bytes())
	epoch.db.SetSync([]byte(latestEpochKey), []byte(strconv.FormatUint(epoch.Number, 10)))

	if epoch.nextEpoch != nil && epoch.nextEpoch.Status == EPOCH_VOTED_NOT_SAVED {
		epoch.nextEpoch.Status = EPOCH_SAVED
		epoch.db.SetSync(calcEpochKeyWithHeight(epoch.nextEpoch.Number), epoch.nextEpoch.Bytes())
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
	fmt.Printf("-------- Next Epoch Info --------\n")
	fmt.Printf("Next epoch proposed: %v", epoch.nextEpoch)

	if epoch.nextEpoch != nil {

		return false
	}

	shouldPropose := curBlockHeight > (epoch.StartBlock+1) && curBlockHeight != epoch.EndBlock
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

			nextEpochVoteSet := epoch.nextEpoch.validatorVoteSet.Copy()

			if nextEpochVoteSet == nil {
				nextEpochVoteSet = NewEpochValidatorVoteSet()
				epoch.logger.Debugf("Should enter new epoch, next epoch vote set is nil, %v", nextEpochVoteSet)
			}

			for i := 0; i < len(newValidators.Validators); i++ {
				v := newValidators.Validators[i]
				vAddr := common.BytesToAddress(v.Address)

				totalProxiedBalance := new(big.Int).Add(state.GetTotalProxiedBalance(vAddr), state.GetTotalDepositProxiedBalance(vAddr))
				newVotingPower := new(big.Int).Add(totalProxiedBalance, state.GetDepositBalance(vAddr))
				if newVotingPower.Sign() == 0 {
					newValidators.Remove(v.Address)

					i--
				} else {
					v.VotingPower = newVotingPower
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
		nextEpoch.previousEpoch = &Epoch{Validators: epoch.Validators}

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
	for i := 0; i < len(validators.Validators); i++ {
		v := validators.Validators[i]
		vAddr := common.BytesToAddress(v.Address)

		totalProxiedBalance := new(big.Int).Add(state.GetTotalProxiedBalance(vAddr), state.GetTotalDepositProxiedBalance(vAddr))
		totalProxiedBalance.Sub(totalProxiedBalance, state.GetTotalPendingRefundBalance(vAddr))

		newVotingPower := new(big.Int).Add(totalProxiedBalance, state.GetDepositBalance(vAddr))
		if newVotingPower.Sign() == 0 {
			validators.Remove(v.Address)
			i--
		} else {
			v.VotingPower = newVotingPower
		}
	}

	if voteSet == nil {
		fmt.Printf("DryRunUpdateEpochValidatorSet, voteSet is nil %v\n", voteSet)
		voteSet = NewEpochValidatorVoteSet()
	}

	_, err := updateEpochValidatorSet(validators, voteSet)
	return err
}

func updateEpochValidatorSet(validators *ncTypes.ValidatorSet, voteSet *EpochValidatorVoteSet) ([]*ncTypes.RefundValidatorAmount, error) {

	var refund []*ncTypes.RefundValidatorAmount
	oldValSize, newValSize := validators.Size(), 0
	fmt.Printf("updateEpochValidatorSet, validators: %v\n, voteSet: %v\n", validators, voteSet)

	if !voteSet.IsEmpty() {
		for _, v := range voteSet.Votes {
			if v.Amount == nil || v.Salt == "" || v.PubKey == nil {
				continue
			}
			_, validator := validators.GetByAddress(v.Address[:])
			if validator == nil {
				added := validators.Add(ncTypes.NewValidator(v.Address[:], v.PubKey, v.Amount))
				if !added {
					fmt.Print(fmt.Errorf("Failed to add new validator %v with voting power %d", v.Address, v.Amount))
				} else {
					newValSize++
				}
			} else {
				if v.Amount.Sign() == 0 {
					fmt.Printf("updateEpochValidatorSet amount is zero\n")
					_, removed := validators.Remove(validator.Address)
					if !removed {
						fmt.Print(fmt.Errorf("Failed to remove validator %v", validator.Address))
					} else {
						refund = append(refund, &ncTypes.RefundValidatorAmount{Address: v.Address, Amount: validator.VotingPower, Voteout: false})
					}
				} else {
					if v.Amount.Cmp(validator.VotingPower) == -1 {
						fmt.Printf("updateEpochValidatorSet amount less than the voting power, amount: %v, votingPower: %v\n", v.Amount, validator.VotingPower)
						refundAmount := new(big.Int).Sub(validator.VotingPower, v.Amount)
						refund = append(refund, &ncTypes.RefundValidatorAmount{Address: v.Address, Amount: refundAmount, Voteout: false})
					}

					validator.VotingPower = v.Amount
					updated := validators.Update(validator)
					if !updated {
						fmt.Print(fmt.Errorf("Failed to update validator %v with voting power %d", validator.Address, v.Amount))
					}
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

func (epoch *Epoch) estimateForNextEpoch(lastBlockHeight uint64, lastBlockTime time.Time) (rewardPerBlock *big.Int, nextEpochBlocks uint64) {

	var rewardFirstYear = epoch.rs.RewardFirstYear
	var epochNumberPerYear = epoch.rs.EpochNumberPerYear
	var totalYear = epoch.rs.TotalMintingYears
	var timePerBlockOfEpoch int64

	const EMERGENCY_BLOCKS_OF_NEXT_EPOCH_LOWER uint64 = 1000
	const EMERGENCY_BLOCKS_OF_NEXT_EPOCH_UPPER uint64 = 5000

	const DEFAULT_TIME_PER_BLOCK_OF_EPOCH int64 = 1000000000

	zeroEpoch := loadOneEpoch(epoch.db, 0, epoch.logger)
	initStartTime := zeroEpoch.StartTime

	thisYear := epoch.Number / epochNumberPerYear
	nextYear := thisYear + 1

	timePerBlockOfEpoch = lastBlockTime.Sub(epoch.StartTime).Nanoseconds() / int64(lastBlockHeight-epoch.StartBlock)

	if timePerBlockOfEpoch <= 0 {
		log.Debugf("estimateForNextEpoch, timePerBlockOfEpoch is %v", timePerBlockOfEpoch)
		timePerBlockOfEpoch = DEFAULT_TIME_PER_BLOCK_OF_EPOCH
	}

	epochLeftThisYear := epochNumberPerYear - epoch.Number%epochNumberPerYear - 1

	nextEpochBlocks = 0

	// log.Info("estimateForNextEpoch",
	// 	"epochLeftThisYear", epochLeftThisYear,
	// 	"timePerBlockOfEpoch", timePerBlockOfEpoch)

	if epochLeftThisYear == 0 {

		nextYearStartTime := initStartTime.AddDate(int(nextYear), 0, 0)

		nextYearEndTime := nextYearStartTime.AddDate(1, 0, 0)

		timeLeftNextYear := nextYearEndTime.Sub(nextYearStartTime)

		epochLeftNextYear := epochNumberPerYear

		epochTimePerEpochLeftNextYear := timeLeftNextYear.Nanoseconds() / int64(epochLeftNextYear)

		nextEpochBlocks = uint64(epochTimePerEpochLeftNextYear / timePerBlockOfEpoch)

		// log.Info("estimateForNextEpoch 0",
		// 	"timePerBlockOfEpoch", timePerBlockOfEpoch,
		// 	"nextYearStartTime", nextYearStartTime,
		// 	"timeLeftNextYear", timeLeftNextYear,
		// 	"epochLeftNextYear", epochLeftNextYear,
		// 	"epochTimePerEpochLeftNextYear", epochTimePerEpochLeftNextYear,
		// 	"nextEpochBlocks", nextEpochBlocks)

		if nextEpochBlocks <= EMERGENCY_BLOCKS_OF_NEXT_EPOCH_LOWER {
			nextEpochBlocks = EMERGENCY_BLOCKS_OF_NEXT_EPOCH_LOWER
			epoch.logger.Warn("EstimateForNextEpoch warning: You should probably take a look at 'epoch_no_per_year' setup in genesis")
		}
		if nextEpochBlocks >= EMERGENCY_BLOCKS_OF_NEXT_EPOCH_UPPER {
			nextEpochBlocks = EMERGENCY_BLOCKS_OF_NEXT_EPOCH_UPPER
			epoch.logger.Warn("EstimateForNextEpoch warning: You should probably take a look at 'epoch_no_per_year' setup in genesis")
		}

		rewardPerEpochNextYear := calculateRewardPerEpochByYear(rewardFirstYear, int64(nextYear), int64(totalYear), int64(epochNumberPerYear))

		rewardPerBlock = new(big.Int).Div(rewardPerEpochNextYear, big.NewInt(int64(nextEpochBlocks)))

	} else {

		nextYearStartTime := initStartTime.AddDate(int(nextYear), 0, 0)

		timeLeftThisYear := nextYearStartTime.Sub(lastBlockTime)

		if timeLeftThisYear > 0 {

			epochTimePerEpochLeftThisYear := timeLeftThisYear.Nanoseconds() / int64(epochLeftThisYear)

			nextEpochBlocks = uint64(epochTimePerEpochLeftThisYear / timePerBlockOfEpoch)

		}

		if nextEpochBlocks <= EMERGENCY_BLOCKS_OF_NEXT_EPOCH_LOWER {
			nextEpochBlocks = EMERGENCY_BLOCKS_OF_NEXT_EPOCH_LOWER
			epoch.logger.Warn("EstimateForNextEpoch warning: You should probably take a look at 'epoch_no_per_year' setup in genesis")
		}
		if nextEpochBlocks > EMERGENCY_BLOCKS_OF_NEXT_EPOCH_UPPER {
			nextEpochBlocks = EMERGENCY_BLOCKS_OF_NEXT_EPOCH_UPPER
			epoch.logger.Warn("EstimateForNextEpoch warning: You should probably take a look at 'epoch_no_per_year' setup in genesis")
		}

		rewardPerEpochThisYear := calculateRewardPerEpochByYear(rewardFirstYear, int64(thisYear), int64(totalYear), int64(epochNumberPerYear))

		rewardPerBlock = new(big.Int).Div(rewardPerEpochThisYear, big.NewInt(int64(nextEpochBlocks)))

	}
	return rewardPerBlock, nextEpochBlocks
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
		// log.Debugf("Epoch equals epoch %v, other %v", epoch, other)
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
	erpb := epoch.RewardPerBlock
	intToFloat := new(big.Float).SetInt(erpb)
	floatToBigFloat := new(big.Float).SetFloat64(1e18)
	var blockReward = new(big.Float).Quo(intToFloat, floatToBigFloat)

	return fmt.Sprintf(
		"Number %v,\n"+
			"Reward per block will be: %v "+"NEAT"+",\n"+
			"Next epoch is starting at block height: %v,\n"+
			"The epoch will last until block height: %v,\n",
		epoch.Number,
		blockReward,
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
