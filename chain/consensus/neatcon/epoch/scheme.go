package epoch

import (
	"fmt"
	"math/big"
	"sync"

	dbm "github.com/neatio-network/db-go"
	ncTypes "github.com/neatio-network/neatio/chain/consensus/neatcon/types"
	"github.com/neatio-network/neatio/chain/log"
	"github.com/neatio-network/wire-go"
)

const rewardSchemeKey = "REWARDSCHEME"

type RewardScheme struct {
	mtx sync.Mutex
	db  dbm.DB

	TotalReward        *big.Int
	RewardFirstYear    *big.Int
	EpochNumberPerYear uint64
	TotalMintingYears  uint64
}

func LoadRewardScheme(db dbm.DB) *RewardScheme {
	buf := db.Get([]byte(rewardSchemeKey))
	if len(buf) == 0 {
		return nil
	} else {
		rs := &RewardScheme{}
		err := wire.ReadBinaryBytes(buf, rs)
		if err != nil {
			log.Errorf("LoadRewardScheme Failed, error: %v", err)
			return nil
		}
		return rs
	}
}

func MakeRewardScheme(db dbm.DB, rsDoc *ncTypes.RewardSchemeDoc) *RewardScheme {

	rs := &RewardScheme{
		db:                 db,
		TotalReward:        rsDoc.TotalReward,
		RewardFirstYear:    rsDoc.RewardFirstYear,
		EpochNumberPerYear: rsDoc.EpochNumberPerYear,
		TotalMintingYears:  rsDoc.TotalMintingYears,
	}

	return rs
}

func (rs *RewardScheme) Save() {
	rs.mtx.Lock()
	defer rs.mtx.Unlock()
	rs.db.SetSync([]byte(rewardSchemeKey), wire.BinaryBytes(*rs))
}

func (rs *RewardScheme) String() string {

	return fmt.Sprintf("RewardScheme : {"+
		"totalReward : %v,\n"+
		"rewardFirstYear : %v,\n"+
		"epochNumberPerYear : %v,\n"+
		"}",
		rs.TotalReward,
		rs.RewardFirstYear,
		rs.EpochNumberPerYear)
}
