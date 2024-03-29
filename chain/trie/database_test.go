package trie

import (
	"testing"

	"github.com/neatio-net/neatio/neatdb/memorydb"
	"github.com/neatio-net/neatio/utilities/common"
)

func TestDatabaseMetarootFetch(t *testing.T) {
	db := NewDatabase(memorydb.New())
	if _, err := db.Node(common.Hash{}); err == nil {
		t.Fatalf("metaroot retrieval succeeded")
	}
}
