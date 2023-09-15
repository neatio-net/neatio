package state

import (
	"fmt"
	"testing"

	"github.com/nio-net/nio/chain/core/rawdb"
	"github.com/nio-net/nio/utilities/common"
)

func TestUpdateCandidateSet(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	state, _ := New(common.Hash{}, NewDatabase(db))

	for i := byte(0); i < 255; i++ {
		addr := common.BytesToAddress([]byte{i})
		state.MarkAddressCandidate(addr)
	}

	state.ClearCandidateSetByAddress(common.BytesToAddress([]byte{byte(1)}))

	canSet := state.candidateSet
	i := 0
	for k, v := range canSet {
		fmt.Printf("index: %v ", i)
		fmt.Printf("candidate set: %v -> %v\n", k, v)
		i++
	}
}
