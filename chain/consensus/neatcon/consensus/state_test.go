package consensus

import (
	"testing"

	"github.com/neatio-network/neatio/chain/consensus/neatcon/types"
	"github.com/neatio-network/neatio/utilities/common/hexutil"
	"github.com/neatlib/crypto-go"
)

var (
	testProposal types.Proposal
	testPubKey   crypto.BLSPubKey
)

func TestVerifyBytes(t *testing.T) {
	testPubKeyBytes, err := hexutil.Decode("0x8A6E0926DC67CD14AB5A6150B5C704F44526AD94CA8DED41220FF302EEBE6DBE36EADFC4CF6246EFBE42F437B4454EA969E848B9CA3FB69B6ABC13450154B5DC4850B7767779D82E60B0C090E2B8F6E1F7EDDAC4828309C2F927A9289848856B787A561A241D3F726B0BAAE87E00D64B09AEB170191C8BE6DA7CCDC1CDAE2F7D")
	if err != nil {
		t.Errorf("decode public key error %v\n", err)
	}

	copy(testPubKey[:], testPubKeyBytes)
}
