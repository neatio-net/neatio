package types

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/neatlab/neatio/utilities/common/hexutil"
	"github.com/neatlib/crypto-go"
	"github.com/neatlib/wire-go"
)

var extraHex = "0x0108696e74636861696e0000000000002c0615cd7d205898b7400000000000000000000201149d2b5214e24104f648819a2b851970e02e0f73c50114dae66c6bca991012f6f62d4db42c79c60e3c2e44010114980aaa734720bdc9d93e3d4030a135aec4910bac0000000000000001011415a43260d301c016d636ec117eac56578f1157990000000000002c06000140439243a34a9ef90323f7db46d7800b7049fa4a50599956054d716de1cf37f7dc686c8a5c6510a17609cda05dd53737cc8635f15da4f7f48ccc1c288db0960e730100000000000000010101000000000000000100"
var extraHex2 = "0x0108696e74636861696e000000000003713d15d1c7c6c2488b8000000000000000000020011415d14546048c69022a12e0c8605338e68a841fb30114ec299e752fd4b41f0ea3d5a4e2b350e6f3a1471b010114ca95018425ce58d2e041eb5d9eda115f181214f800000000000000010114f4fd5d4e44794f67fa797f7824dd706934970e3c000000000003713d010201405678a97feace6079d60753d636ad7358349fcd2b222e1ffff393359f2c272e5d132f435b18967010e881db0dc52055c8c50ff0ac660195c494aa85a7015c7de30100000000000000030101000000000000000300"
var blsPubkeyHex = "0x31A9A1B8808146B846E8D919F9BF565F3F18E1F5359101B05BB41A28761F671411D824C15794553ACC74E85E5B6E67919B7C4CCFB844AC21611DD9AD2B78AA2069B988F89E9C2C6DA57A68960584D0346FFD910823DE06C55D7B151AECBBC4731F76DDA5E7202BB1EAF824F065EB43E187D14CC1EBC5C69033D42D03F699D8F2"

func TestEncodeNeatConExtra(t *testing.T) {
	var extra = NeatConExtra{}
	extra = NeatConExtra{
		ChainID:         "neatio",
		Height:          uint64(11270),
		Time:            time.Now(),
		NeedToSave:      false,
		NeedToBroadcast: false,
		EpochNumber:     uint64(2),
		SeenCommitHash:  []byte{0x3e, 0x78, 0x73, 0xef, 0xaf, 0x71, 0x6, 0xda, 0x71, 0x62, 0x68, 0xbe, 0x31, 0xd2, 0x73, 0xf, 0xa4, 0x28, 0x35, 0xda},
		ValidatorsHash:  []byte{0xda, 0xe6, 0x6c, 0x6b, 0xca, 0x99, 0x10, 0x12, 0xf6, 0xf6, 0x2d, 0x4d, 0xb4, 0x2c, 0x79, 0xc6, 0xe, 0x3c, 0x2e, 0x44},
		SeenCommit:      &Commit{},
		EpochBytes:      []byte{},
	}
	extraHash := extra.Hash()
	fmt.Printf("extraHash=%v\n", hex.EncodeToString(extraHash))

}

func TestValidator_Hash(t *testing.T) {
	var blsPubKey crypto.BLSPubKey
	blsPubKeyByte, err := hexutil.Decode(blsPubkeyHex)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("blsPubKeyByte len=%v\n", len(blsPubKeyByte))
	for i, v := range blsPubKeyByte {
		blsPubKey[i] = v
	}
	fmt.Printf("blsPubKey%v\n", blsPubKey)
	validator := Validator{
		Address:        []byte("32H8py5Jg396p7QNDUwTwkeVod15ksxne5"),
		PubKey:         blsPubKey,
		VotingPower:    big.NewInt(10000000000),
		RemainingEpoch: uint64(0),
	}

	validatorHash := validator.Hash()
	fmt.Printf("validatorHash=%v\n", validatorHash)
	fmt.Printf("validatorHashHex=%v\n", hexutil.Encode(validatorHash))
}

func TestDecodeNeatConExtra(t *testing.T) {
	var extra = NeatConExtra{}
	extraByte, err := hexutil.Decode(extraHex2)
	fmt.Printf("extraByte=%v\n\n", extraByte)
	if err != nil {
		t.Error(err)
	}
	err = wire.ReadBinaryBytes(extraByte, &extra)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf("extra=%v\n", extra)
	fmt.Printf("extra.validatorshash=%v\n", extra.ValidatorsHash)
	fmt.Printf("extra.validatorshashhex=%v\n\n", hexutil.Encode(extra.ValidatorsHash))
	fmt.Printf("extra.SeenCommit=%v\n", *extra.SeenCommit)
	fmt.Printf("extra.SeenCommit.BitArray=%v\n", extra.SeenCommit.BitArray)
	fmt.Printf("extra.SeenCommit.NumCommits=%v\n", extra.SeenCommit.NumCommits())
}
