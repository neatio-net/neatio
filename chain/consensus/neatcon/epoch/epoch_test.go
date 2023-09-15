package epoch

import (
	"fmt"
	"math/big"
	"sort"
	"testing"
	"time"

	"github.com/nio-net/nio/utilities/common"
)

func TestEstimateEpoch(t *testing.T) {
	timeA := time.Now()
	timeB := timeA.Unix()
	timeStr := timeA.String()
	t.Logf("now: %v, %v", timeB, timeStr)

	formatTimeStr := "2021-12-20 09:51:38.397502 +0800 CST m=+10323.270024761"
	parse, e := time.Parse("", formatTimeStr)
	if e == nil {
		t.Logf("time: %v", parse)
	} else {
		t.Errorf("parse error: %v", e)
	}

	timeC := time.Now().UnixNano()
	t.Logf("time c %v", timeC)
	t.Logf("time c %v", timeC)
}

func TestVoteSetCompare(t *testing.T) {
	var voteArr []*EpochValidatorVote
	voteArr = []*EpochValidatorVote{
		{
			Address: common.StringToAddress("NEATfxRkRYDgG5gjaJSTpwZz9cZQsMEr"),
			Amount:  big.NewInt(1),
		},
		{
			Address: common.StringToAddress("NEAToT41aVcZXxKxY5F8nMY6MU6AdsVx"),
			Amount:  big.NewInt(1),
		},
		{
			Address: common.StringToAddress("NEATqSts5DJr6pwKVdDefhHEYvbWUPGD"),
			Amount:  big.NewInt(1),
		},
		{
			Address: common.StringToAddress("NEATxUxwDzVQrRKB9WZwfoUFiJY9dizi"),
			Amount:  big.NewInt(1),
		},
		{
			Address: common.StringToAddress("NEATcbjuDR3PGRghZdTduYpykCFhZBS2"),
			Amount:  big.NewInt(1),
		},
		{
			Address: common.StringToAddress("NEATgAEfFuiMug54tpoh6GPDZ8SNB6a6"),
			Amount:  big.NewInt(1),
		},
	}

	sort.Slice(voteArr, func(i, j int) bool {
		if voteArr[i].Amount.Cmp(voteArr[j].Amount) == 0 {
			return compareAddress(voteArr[i].Address[:], voteArr[j].Address[:])
		}

		return voteArr[i].Amount.Cmp(voteArr[j].Amount) == 1
	})
	for i := range voteArr {
		fmt.Printf("address:%v, amount: %v\n", voteArr[i].Address.String(), voteArr[i].Amount)
	}
}
