// +build none

// This program generates contract/code.go, which contains the chequebook code
// after deployment.
package main

import (
	"math/big"

	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/utilities/crypto"
)

var (
	testKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAlloc  = core.GenesisAlloc{
		crypto.PubkeyToAddress(testKey.PublicKey): {Balance: big.NewInt(500000000000)},
	}
)
