package neatptc

import (
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"
	"sort"
	"sync"
	"testing"

	"github.com/neatio-net/neatio/chain/core"
	"github.com/neatio-net/neatio/chain/core/rawdb"
	"github.com/neatio-net/neatio/chain/core/types"
	"github.com/neatio-net/neatio/chain/core/vm"
	"github.com/neatio-net/neatio/neatdb"
	"github.com/neatio-net/neatio/neatptc/downloader"
	"github.com/neatio-net/neatio/network/p2p"
	"github.com/neatio-net/neatio/network/p2p/discover"
	"github.com/neatio-net/neatio/params"
	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/crypto"
	"github.com/neatio-net/neatio/utilities/event"
)

var (
	testBankKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testBank       = crypto.PubkeyToAddress(testBankKey.PublicKey)
)

func newTestProtocolManager(mode downloader.SyncMode, blocks int, generator func(int, *core.BlockGen), newtx chan<- []*types.Transaction) (*ProtocolManager, neatdb.Database, error) {
	var (
		evmux = new(event.TypeMux)
		db    = rawdb.NewMemoryDatabase()
		gspec = &core.Genesis{
			Config: params.TestChainConfig,
			Alloc:  core.GenesisAlloc{testBank: {Balance: big.NewInt(1000000)}},
		}
		genesis       = gspec.MustCommit(db)
		blockchain, _ = core.NewBlockChain(db, nil, gspec.Config, nil, vm.Config{}, nil)
	)
	chain, _ := core.GenerateChain(gspec.Config, genesis, nil, db, blocks, generator)
	if _, err := blockchain.InsertChain(chain); err != nil {
		panic(err)
	}

	pm, err := NewProtocolManager(gspec.Config, mode, DefaultConfig.NetworkId, evmux, &testTxPool{added: newtx}, engine, blockchain, db, nil)
	if err != nil {
		return nil, nil, err
	}
	pm.Start(1000)
	return pm, db, nil
}

func newTestProtocolManagerMust(t *testing.T, mode downloader.SyncMode, blocks int, generator func(int, *core.BlockGen), newtx chan<- []*types.Transaction) (*ProtocolManager, neatdb.Database) {
	pm, db, err := newTestProtocolManager(mode, blocks, generator, newtx)
	if err != nil {
		t.Fatalf("Failed to create protocol manager: %v", err)
	}
	return pm, db
}

type testTxPool struct {
	txFeed event.Feed
	pool   []*types.Transaction
	added  chan<- []*types.Transaction

	lock sync.RWMutex
}

func (p *testTxPool) AddRemotes(txs []*types.Transaction) []error {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool = append(p.pool, txs...)
	if p.added != nil {
		p.added <- txs
	}
	return make([]error, len(txs))
}

func (p *testTxPool) Pending() (map[common.Address]types.Transactions, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	batches := make(map[common.Address]types.Transactions)
	for _, tx := range p.pool {
		from, _ := types.Sender(types.HomesteadSigner{}, tx)
		batches[from] = append(batches[from], tx)
	}
	for _, batch := range batches {
		sort.Sort(types.TxByNonce(batch))
	}
	return batches, nil
}

func (p *testTxPool) SubscribeTxPreEvent(ch chan<- core.TxPreEvent) event.Subscription {
	return p.txFeed.Subscribe(ch)
}

func newTestTransaction(from *ecdsa.PrivateKey, nonce uint64, datasize int) *types.Transaction {
	tx := types.NewTransaction(nonce, common.Address{}, big.NewInt(0), 100000, big.NewInt(0), make([]byte, datasize))
	tx, _ = types.SignTx(tx, types.HomesteadSigner{}, from)
	return tx
}

type testPeer struct {
	net p2p.MsgReadWriter
	app *p2p.MsgPipeRW
	*peer
}

func newTestPeer(name string, version int, pm *ProtocolManager, shake bool) (*testPeer, <-chan error) {

	app, net := p2p.MsgPipe()

	var id discover.NodeID
	rand.Read(id[:])

	peer := pm.newPeer(version, p2p.NewPeer(id, name, nil), net)

	errc := make(chan error, 1)
	go func() {
		select {
		case pm.newPeerCh <- peer:
			errc <- pm.handle(peer)
		case <-pm.quitSync:
			errc <- p2p.DiscQuitting
		}
	}()
	tp := &testPeer{app: app, net: net, peer: peer}

	if shake {
		var (
			genesis = pm.blockchain.Genesis()
			head    = pm.blockchain.CurrentHeader()
			td      = pm.blockchain.GetTd(head.Hash(), head.Number.Uint64())
		)
		tp.handshake(nil, td, head.Hash(), genesis.Hash())
	}
	return tp, errc
}

func (p *testPeer) handshake(t *testing.T, td *big.Int, head common.Hash, genesis common.Hash) {
	msg := &statusData{
		ProtocolVersion: uint32(p.version),
		NetworkId:       DefaultConfig.NetworkId,
		TD:              td,
		CurrentBlock:    head,
		GenesisBlock:    genesis,
	}
	if err := p2p.ExpectMsg(p.app, StatusMsg, msg); err != nil {
		t.Fatalf("status recv: %v", err)
	}
	if err := p2p.Send(p.app, StatusMsg, msg); err != nil {
		t.Fatalf("status send: %v", err)
	}
}

func (p *testPeer) close() {
	p.app.Close()
}
