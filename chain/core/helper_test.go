package core

import (
	"container/list"

	"github.com/nio-net/nio/chain/core/rawdb"
	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/neatdb"
	"github.com/nio-net/nio/utilities/event"
)

type TestManager struct {
	eventMux *event.TypeMux

	db         neatdb.Database
	txPool     *TxPool
	blockChain *BlockChain
	Blocks     []*types.Block
}

func (tm *TestManager) IsListening() bool {
	return false
}

func (tm *TestManager) IsMining() bool {
	return false
}

func (tm *TestManager) PeerCount() int {
	return 0
}

func (tm *TestManager) Peers() *list.List {
	return list.New()
}

func (tm *TestManager) BlockChain() *BlockChain {
	return tm.blockChain
}

func (tm *TestManager) TxPool() *TxPool {
	return tm.txPool
}

func (tm *TestManager) EventMux() *event.TypeMux {
	return tm.eventMux
}

func (tm *TestManager) Db() neatdb.Database {
	return tm.db
}

func NewTestManager() *TestManager {

	testManager := &TestManager{}
	testManager.eventMux = new(event.TypeMux)
	testManager.db = rawdb.NewMemoryDatabase()

	return testManager
}
