package types

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/neatio-net/neatio/utilities/common/hexutil"
	"github.com/neatio-net/neatio/utilities/crypto"
)

func TestLoose23MajorThreshold(t *testing.T) {
	totalVP := big.NewInt(10)
	round := 0

	quroum := Loose23MajorThreshold(totalVP, round)
	t.Logf("Loose 2/3 major threshold %v", quroum)

	stringByte := []byte("love")

	encode := hexutil.Encode(stringByte)

	hash := crypto.Keccak256Hash(stringByte)

	fmt.Printf("ecnode data %v\n", encode)
	fmt.Printf("ecnode hash %v\n", hash.String())
}
