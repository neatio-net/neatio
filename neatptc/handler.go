package neatptc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nio-net/nio/chain/core/rawdb"

	"github.com/nio-net/nio/chain/consensus"
	"github.com/nio-net/nio/chain/core"
	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/neatdb"
	"github.com/nio-net/nio/neatptc/downloader"
	"github.com/nio-net/nio/neatptc/fetcher"
	"github.com/nio-net/nio/network/p2p"
	"github.com/nio-net/nio/network/p2p/discover"
	"github.com/nio-net/nio/params"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/crypto"
	"github.com/nio-net/nio/utilities/event"
	"github.com/nio-net/nio/utilities/rlp"
)

const (
	softResponseLimit = 2 * 1024 * 1024
	estHeaderRlpSize  = 500

	txChanSize = 4096

	tx3PrfDtChainSize = 4096
)

var (
	daoChallengeTimeout = 15 * time.Second
)

var errIncompatibleConfig = errors.New("incompatible configuration")

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

type ProtocolManager struct {
	networkId uint64

	fastSync  uint32
	acceptTxs uint32

	txpool      txPool
	blockchain  *core.BlockChain
	chainconfig *params.ChainConfig
	maxPeers    int

	downloader *downloader.Downloader
	fetcher    *fetcher.Fetcher
	peers      *peerSet

	SubProtocols []p2p.Protocol

	eventMux *event.TypeMux
	txCh     chan core.TxPreEvent
	txSub    event.Subscription

	tx3PrfDtCh    chan core.Tx3ProofDataEvent
	tx3PrfDtFeed  event.Feed
	tx3PrfDtScope event.SubscriptionScope
	tx3PrfDtSub   event.Subscription

	minedBlockSub *event.TypeMuxSubscription

	newPeerCh   chan *peer
	txsyncCh    chan *txsync
	quitSync    chan struct{}
	noMorePeers chan struct{}

	wg sync.WaitGroup

	engine consensus.Engine

	cch core.CrossChainHelper

	logger         log.Logger
	preimageLogger log.Logger
}

func NewProtocolManager(config *params.ChainConfig, mode downloader.SyncMode, networkId uint64, mux *event.TypeMux, txpool txPool, engine consensus.Engine, blockchain *core.BlockChain, chaindb neatdb.Database, cch core.CrossChainHelper) (*ProtocolManager, error) {

	manager := &ProtocolManager{
		networkId:      networkId,
		eventMux:       mux,
		txpool:         txpool,
		blockchain:     blockchain,
		chainconfig:    config,
		peers:          newPeerSet(),
		newPeerCh:      make(chan *peer),
		noMorePeers:    make(chan struct{}),
		txsyncCh:       make(chan *txsync),
		quitSync:       make(chan struct{}),
		engine:         engine,
		cch:            cch,
		logger:         config.ChainLogger,
		preimageLogger: config.ChainLogger.New("module", "preimages"),
	}

	if handler, ok := manager.engine.(consensus.Handler); ok {
		handler.SetBroadcaster(manager)
	}

	if mode == downloader.FastSync && blockchain.CurrentBlock().NumberU64() > 0 {
		manager.logger.Warn("Blockchain not empty, fast sync disabled")
		mode = downloader.FullSync
	}
	if mode == downloader.FastSync {
		manager.fastSync = uint32(1)
	}
	protocol := engine.Protocol()

	manager.SubProtocols = make([]p2p.Protocol, 0, len(protocol.Versions))
	for i, version := range protocol.Versions {

		if mode == downloader.FastSync && version < consensus.Eth63 {
			continue
		}

		version := version
		manager.SubProtocols = append(manager.SubProtocols, p2p.Protocol{
			Name:    protocol.Name,
			Version: version,
			Length:  protocol.Lengths[i],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := manager.newPeer(int(version), p, rw)
				select {
				case manager.newPeerCh <- peer:
					manager.wg.Add(1)
					defer manager.wg.Done()
					return manager.handle(peer)
				case <-manager.quitSync:
					return p2p.DiscQuitting
				}
			},
			NodeInfo: func() interface{} {
				return manager.NodeInfo()
			},
			PeerInfo: func(id discover.NodeID) interface{} {
				if p := manager.peers.Peer(fmt.Sprintf("%x", id[:8])); p != nil {
					return p.Info()
				}
				return nil
			},
		})
	}
	if len(manager.SubProtocols) == 0 {
		return nil, errIncompatibleConfig
	}

	manager.downloader = downloader.New(mode, chaindb, manager.eventMux, blockchain, nil, manager.removePeer, manager.logger)

	validator := func(header *types.Header) error {
		return engine.VerifyHeader(blockchain, header, true)
	}
	heighter := func() uint64 {
		return blockchain.CurrentBlock().NumberU64()
	}
	inserter := func(blocks types.Blocks) (int, error) {

		if atomic.LoadUint32(&manager.fastSync) == 1 {
			manager.logger.Warn("Discarded bad propagated block", "number", blocks[0].Number(), "hash", blocks[0].Hash())
			return 0, nil
		}
		atomic.StoreUint32(&manager.acceptTxs, 1)
		return manager.blockchain.InsertChain(blocks)
	}
	manager.fetcher = fetcher.New(blockchain.GetBlockByHash, validator, manager.BroadcastBlock, heighter, inserter, manager.removePeer)

	return manager, nil
}

func (pm *ProtocolManager) removePeer(id string) {

	peer := pm.peers.Peer(id)
	if peer == nil {
		return
	}
	pm.logger.Debug("Removing NEAT Blockchain peer", "peer", id)

	pm.downloader.UnregisterPeer(id)
	if err := pm.peers.Unregister(id); err != nil {
		pm.logger.Error("Peer removal failed", "peer", id, "err", err)
	}

	if peer != nil {
		peer.Peer.Disconnect(p2p.DiscUselessPeer)
	}
}

func (pm *ProtocolManager) Start(maxPeers int) {
	pm.maxPeers = maxPeers

	pm.txCh = make(chan core.TxPreEvent, txChanSize)
	pm.txSub = pm.txpool.SubscribeTxPreEvent(pm.txCh)
	go pm.txBroadcastLoop()

	pm.tx3PrfDtCh = make(chan core.Tx3ProofDataEvent, tx3PrfDtChainSize)
	pm.tx3PrfDtSub = pm.tx3PrfDtScope.Track(pm.tx3PrfDtFeed.Subscribe(pm.tx3PrfDtCh))
	go pm.tx3PrfDtBroadcastLoop()

	pm.minedBlockSub = pm.eventMux.Subscribe(core.NewMinedBlockEvent{})
	go pm.minedBroadcastLoop()

	go pm.syncer()
	go pm.txsyncLoop()
}

func (pm *ProtocolManager) Stop() {
	pm.logger.Info("Stopping Neatio protocol")

	pm.txSub.Unsubscribe()
	pm.tx3PrfDtSub.Unsubscribe()
	pm.minedBlockSub.Unsubscribe()

	pm.noMorePeers <- struct{}{}

	close(pm.quitSync)

	pm.peers.Close()

	pm.wg.Wait()

	pm.logger.Info("Neatio protocol stopped")
}

func (pm *ProtocolManager) newPeer(pv int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	return newPeer(pv, p, newMeteredMsgWriter(rw))
}

func (pm *ProtocolManager) handle(p *peer) error {

	if pm.peers.Len() >= pm.maxPeers && !p.Peer.Info().Network.Trusted {
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("NEAT Blockchain peer connected", "name", p.Name())

	var (
		genesis = pm.blockchain.Genesis()
		head    = pm.blockchain.CurrentHeader()
		hash    = head.Hash()
		number  = head.Number.Uint64()
		td      = pm.blockchain.GetTd(hash, number)
	)
	if err := p.Handshake(pm.networkId, td, hash, genesis.Hash()); err != nil {
		p.Log().Debug("NEAT Blockchain handshake failed", "err", err)
		return err
	}
	if rw, ok := p.rw.(*meteredMsgReadWriter); ok {
		rw.Init(p.version)
	}

	if err := pm.peers.Register(p); err != nil {
		p.Log().Error("NEAT Blockchain peer registration failed", "err", err)
		return err
	}

	defer func() {
		pm.removePeer(p.id)
		if handler, ok := pm.engine.(consensus.Handler); ok {
			handler.RemovePeer(p)
		}
	}()

	if err := pm.downloader.RegisterPeer(p.id, p.version, p); err != nil {
		return err
	}

	pm.syncTransactions(p)

	if handler, ok := pm.engine.(consensus.Handler); ok {
		handler.AddPeer(p)
	} else {
		p.Log().Info("AddPeer not executed")
	}

	for {
		if err := pm.handleMsg(p); err != nil {
			p.Log().Debug("NEAT Blockchain message handling failed", "err", err)
			return err
		}
	}
}

func (pm *ProtocolManager) handleMsg(p *peer) error {

	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	switch {

	case msg.Code >= 0x20 && msg.Code <= 0x23:
		if handler, ok := pm.engine.(consensus.Handler); ok {
			var msgBytes []byte
			if err := msg.Decode(&msgBytes); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			handler.HandleMsg(msg.Code, p, msgBytes)
		}
	case msg.Code == StatusMsg:

		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

	case msg.Code == GetBlockHeadersMsg:

		var query getBlockHeadersData
		if err := msg.Decode(&query); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		hashMode := query.Origin.Hash != (common.Hash{})

		var (
			bytes   common.StorageSize
			headers []*types.Header
			unknown bool
		)
		for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {

			var origin *types.Header
			if hashMode {
				origin = pm.blockchain.GetHeaderByHash(query.Origin.Hash)
			} else {
				origin = pm.blockchain.GetHeaderByNumber(query.Origin.Number)
			}
			if origin == nil {
				break
			}
			number := origin.Number.Uint64()
			headers = append(headers, origin)
			bytes += estHeaderRlpSize

			switch {
			case query.Origin.Hash != (common.Hash{}) && query.Reverse:

				for i := 0; i < int(query.Skip)+1; i++ {
					if header := pm.blockchain.GetHeader(query.Origin.Hash, number); header != nil {
						query.Origin.Hash = header.ParentHash
						number--
					} else {
						unknown = true
						break
					}
				}
			case query.Origin.Hash != (common.Hash{}) && !query.Reverse:

				var (
					current = origin.Number.Uint64()
					next    = current + query.Skip + 1
				)
				if next <= current {
					infos, _ := json.MarshalIndent(p.Peer.Info(), "", "  ")
					p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
					unknown = true
				} else {
					if header := pm.blockchain.GetHeaderByNumber(next); header != nil {
						if pm.blockchain.GetBlockHashesFromHash(header.Hash(), query.Skip+1)[query.Skip] == query.Origin.Hash {
							query.Origin.Hash = header.Hash()
						} else {
							unknown = true
						}
					} else {
						unknown = true
					}
				}
			case query.Reverse:

				if query.Origin.Number >= query.Skip+1 {
					query.Origin.Number -= query.Skip + 1
				} else {
					unknown = true
				}

			case !query.Reverse:

				query.Origin.Number += query.Skip + 1
			}
		}
		return p.SendBlockHeaders(headers)

	case msg.Code == BlockHeadersMsg:

		var headers []*types.Header
		if err := msg.Decode(&headers); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		filter := len(headers) == 1
		if filter {

			headers = pm.fetcher.FilterHeaders(p.id, headers, time.Now())
		}
		if len(headers) > 0 || !filter {
			err := pm.downloader.DeliverHeaders(p.id, headers)
			if err != nil {
				pm.logger.Debug("Failed to deliver headers", "err", err)
			}
		}

	case msg.Code == GetBlockBodiesMsg:

		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}

		var (
			hash   common.Hash
			bytes  int
			bodies []rlp.RawValue
		)
		for bytes < softResponseLimit && len(bodies) < downloader.MaxBlockFetch {

			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}

			if data := pm.blockchain.GetBodyRLP(hash); len(data) != 0 {
				bodies = append(bodies, data)
				bytes += len(data)
			}
		}
		return p.SendBlockBodiesRLP(bodies)

	case msg.Code == BlockBodiesMsg:

		var request blockBodiesData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		trasactions := make([][]*types.Transaction, len(request))
		uncles := make([][]*types.Header, len(request))

		for i, body := range request {
			trasactions[i] = body.Transactions
			uncles[i] = body.Uncles
		}

		filter := len(trasactions) > 0 || len(uncles) > 0
		if filter {
			trasactions, uncles = pm.fetcher.FilterBodies(p.id, trasactions, uncles, time.Now())
		}
		if len(trasactions) > 0 || len(uncles) > 0 || !filter {
			err := pm.downloader.DeliverBodies(p.id, trasactions, uncles)
			if err != nil {
				pm.logger.Debug("Failed to deliver bodies", "err", err)
			}
		}

	case p.version >= consensus.Eth63 && msg.Code == GetNodeDataMsg:

		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}

		var (
			hash  common.Hash
			bytes int
			data  [][]byte
		)
		for bytes < softResponseLimit && len(data) < downloader.MaxStateFetch {

			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}

			if entry, err := pm.blockchain.TrieNode(hash); err == nil {
				data = append(data, entry)
				bytes += len(entry)
			}
		}
		return p.SendNodeData(data)

	case p.version >= consensus.Eth63 && msg.Code == NodeDataMsg:

		var data [][]byte
		if err := msg.Decode(&data); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		if err := pm.downloader.DeliverNodeData(p.id, data); err != nil {
			pm.logger.Debug("Failed to deliver node state data", "err", err)
		}

	case p.version >= consensus.Eth63 && msg.Code == GetReceiptsMsg:

		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}

		var (
			hash     common.Hash
			bytes    int
			receipts []rlp.RawValue
		)
		for bytes < softResponseLimit && len(receipts) < downloader.MaxReceiptFetch {

			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}

			results := pm.blockchain.GetReceiptsByHash(hash)
			if results == nil {
				if header := pm.blockchain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
					continue
				}
			}

			if encoded, err := rlp.EncodeToBytes(results); err != nil {
				pm.logger.Error("Failed to encode receipt", "err", err)
			} else {
				receipts = append(receipts, encoded)
				bytes += len(encoded)
			}
		}
		return p.SendReceiptsRLP(receipts)

	case p.version >= consensus.Eth63 && msg.Code == ReceiptsMsg:

		var receipts [][]*types.Receipt
		if err := msg.Decode(&receipts); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		if err := pm.downloader.DeliverReceipts(p.id, receipts); err != nil {
			pm.logger.Debug("Failed to deliver receipts", "err", err)
		}

	case msg.Code == NewBlockHashesMsg:
		var announces newBlockHashesData
		if err := msg.Decode(&announces); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}

		for _, block := range announces {
			p.MarkBlock(block.Hash)
		}

		unknown := make(newBlockHashesData, 0, len(announces))
		for _, block := range announces {
			if !pm.blockchain.HasBlock(block.Hash, block.Number) {
				unknown = append(unknown, block)
			}
		}
		for _, block := range unknown {
			pm.fetcher.Notify(p.id, block.Hash, block.Number, time.Now(), p.RequestOneHeader, p.RequestBodies)
		}

	case msg.Code == NewBlockMsg:

		var request newBlockData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		request.Block.ReceivedAt = msg.ReceivedAt
		request.Block.ReceivedFrom = p

		p.MarkBlock(request.Block.Hash())
		pm.fetcher.Enqueue(p.id, request.Block)

		var (
			trueHead = request.Block.ParentHash()
			trueTD   = new(big.Int).Sub(request.TD, request.Block.Difficulty())
		)

		if _, td := p.Head(); trueTD.Cmp(td) > 0 {
			p.SetHead(trueHead, trueTD)

			currentBlock := pm.blockchain.CurrentBlock()
			if trueTD.Cmp(pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())) > 0 {
				go pm.synchronise(p)
			}
		}

	case msg.Code == TxMsg:

		if atomic.LoadUint32(&pm.acceptTxs) == 0 {
			break
		}

		var txs []*types.Transaction
		if err := msg.Decode(&txs); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		for i, tx := range txs {

			if tx == nil {
				return errResp(ErrDecode, "transaction %d is nil", i)
			}
			p.MarkTransaction(tx.Hash())
		}
		pm.txpool.AddRemotes(txs)

	case msg.Code == TX3ProofDataMsg:
		pm.logger.Debug("TX3ProofDataMsg received")
		var proofDatas []*types.TX3ProofData
		if err := msg.Decode(&proofDatas); err != nil {
			pm.logger.Error("TX3ProofDataMsg decode error", "msg", msg, "error", err)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		for _, proofData := range proofDatas {

			if err := pm.cch.ValidateTX3ProofData(proofData); err != nil {
				pm.logger.Error("TX3ProofDataMsg validate error", "msg", msg, "error", err)
				return errResp(ErrTX3ValidateFail, "msg %v: %v", msg, err)
			}
			p.MarkTX3ProofData(proofData.Header.Hash())

			if err := pm.cch.WriteTX3ProofData(proofData); err != nil {
				pm.logger.Error("TX3ProofDataMsg write error", "msg", msg, "error", err)
			}

			go pm.tx3PrfDtFeed.Send(core.Tx3ProofDataEvent{proofData})
		}

	case msg.Code == GetPreImagesMsg:
		pm.preimageLogger.Debug("GetPreImagesMsg received")

		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}

		var (
			hash      common.Hash
			bytes     int
			preimages [][]byte
		)

		for bytes < softResponseLimit && len(preimages) < downloader.MaxReceiptFetch {

			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {

				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}

			preimage := rawdb.ReadPreimage(pm.blockchain.StateCache().TrieDB().DiskDB(), hash)

			if hash != crypto.Keccak256Hash(preimage) {
				pm.preimageLogger.Errorf("Failed to pass the preimage double check. Request hash %x, Local Preimage %x", hash, preimage)
				continue
			}

			preimages = append(preimages, preimage)
			bytes += len(preimage)
		}
		return p.SendPreimagesRLP(preimages)

	case msg.Code == PreImagesMsg:
		pm.preimageLogger.Debug("PreImagesMsg received")
		var preimages [][]byte
		if err := msg.Decode(&preimages); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		preimagesMap := make(map[common.Hash][]byte)
		for _, preimage := range preimages {
			pm.preimageLogger.Debugf("PreImagesMsg received: %x", preimage)
			preimagesMap[crypto.Keccak256Hash(preimage)] = common.CopyBytes(preimage)
		}
		if len(preimagesMap) > 0 {
			db, _ := pm.blockchain.StateCache().TrieDB().DiskDB().(neatdb.Database)
			rawdb.WritePreimages(db, preimagesMap)
			pm.preimageLogger.Info("PreImages wrote into database")
		}
	case msg.Code == TrieNodeDataMsg:
		pm.logger.Debug("TrieNodeDataMsg received")
		var trienodes [][]byte
		if err := msg.Decode(&trienodes); err != nil {
			pm.logger.Warnf("Unable decode TrieNodeData %v", err)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		pm.logger.Debugf("%d TrieNodeData received", len(trienodes))

		db, _ := pm.blockchain.StateCache().TrieDB().DiskDB().(neatdb.Database)
		for _, tnode := range trienodes {
			thash := crypto.Keccak256Hash(tnode)
			if has, herr := db.Has(thash.Bytes()); !has && herr == nil {
				puterr := db.Put(thash.Bytes(), tnode)
				if puterr == nil {
					pm.logger.Debugf("Insert TrieNodeData %x", thash)
				}
			} else if has {
				pm.logger.Debugf("TrieNodeData %x already existed", thash)
			}
		}
	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}

func (pm *ProtocolManager) Enqueue(id string, block *types.Block) {
	pm.fetcher.Enqueue(id, block)
}

func (pm *ProtocolManager) BroadcastBlock(block *types.Block, propagate bool) {
	hash := block.Hash()
	peers := pm.peers.PeersWithoutBlock(hash)

	if propagate {

		var td *big.Int
		if parent := pm.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1); parent != nil {
			td = new(big.Int).Add(block.Difficulty(), pm.blockchain.GetTd(block.ParentHash(), block.NumberU64()-1))
		} else {
			pm.logger.Error("Propagating dangling block", "number", block.Number(), "hash", hash)
			return
		}

		transfer := peers[:int(math.Sqrt(float64(len(peers))))]
		for _, peer := range transfer {
			peer.SendNewBlock(block, td)
		}
		pm.logger.Trace("Propagated block", "hash", hash, "recipients", len(transfer), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
		return
	}

	if pm.blockchain.HasBlock(hash, block.NumberU64()) {
		for _, peer := range peers {
			peer.SendNewBlockHashes([]common.Hash{hash}, []uint64{block.NumberU64()})
		}
		pm.logger.Trace("Announced block", "hash", hash, "recipients", len(peers), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
	}
}

func (pm *ProtocolManager) BroadcastTx(hash common.Hash, tx *types.Transaction) {

	peers := pm.peers.PeersWithoutTx(hash)

	for _, peer := range peers {
		peer.SendTransactions(types.Transactions{tx})
	}
	pm.logger.Trace("Broadcast transaction", "hash", hash, "recipients", len(peers))
}

func (pm *ProtocolManager) BroadcastTX3ProofData(hash common.Hash, proofData *types.TX3ProofData) {

	peers := pm.peers.PeersWithoutTX3ProofData(hash)
	for _, peer := range peers {
		peer.SendTX3ProofData([]*types.TX3ProofData{proofData})
	}
	pm.logger.Trace("Broadcast TX3ProofData", "hash", hash, "recipients", len(peers))
}

func (pm *ProtocolManager) BroadcastMessage(msgcode uint64, data interface{}) {
	recipients := 0
	for _, peer := range pm.peers.Peers() {
		peer.Send(msgcode, data)
		recipients++
	}
	pm.logger.Trace("Broadcast p2p message", "code", msgcode, "recipients", recipients, "msg", data)
}

func (pm *ProtocolManager) TryFixBadPreimages() {

	images := make(map[common.Hash][]byte)

	var hashes []common.Hash

	db, _ := pm.blockchain.StateCache().TrieDB().DiskDB().(neatdb.Database)
	it := db.NewIteratorWithPrefix([]byte("secure-key-"))
	for it.Next() {
		keyHash := common.BytesToHash(it.Key())
		valueHash := crypto.Keccak256Hash(it.Value())
		if keyHash != valueHash {

			hashes = append(hashes, keyHash)
		}

		images[keyHash] = common.CopyBytes(it.Value())
	}
	it.Release()

	if len(hashes) > 0 {
		pm.preimageLogger.Critf("Found %d Bad Preimage(s)", len(hashes))
		pm.preimageLogger.Critf("Bad Preimages: %x", hashes)

		pm.peers.BestPeer().RequestPreimages(hashes)
	}

}

func (self *ProtocolManager) minedBroadcastLoop() {

	for obj := range self.minedBlockSub.Chan() {
		switch ev := obj.Data.(type) {
		case core.NewMinedBlockEvent:
			self.BroadcastBlock(ev.Block, true)
			self.BroadcastBlock(ev.Block, false)
		}
	}
}

func (self *ProtocolManager) txBroadcastLoop() {
	for {
		select {
		case event := <-self.txCh:
			self.BroadcastTx(event.Tx.Hash(), event.Tx)

		case <-self.txSub.Err():
			return
		}
	}
}

func (self *ProtocolManager) tx3PrfDtBroadcastLoop() {
	for {
		select {
		case event := <-self.tx3PrfDtCh:
			self.BroadcastTX3ProofData(event.Tx3PrfDt.Header.Hash(), event.Tx3PrfDt)

		case <-self.tx3PrfDtSub.Err():
			return
		}
	}
}

type NodeInfo struct {
	Network    uint64              `json:"network"`
	Difficulty *big.Int            `json:"difficulty"`
	Genesis    common.Hash         `json:"genesis"`
	Config     *params.ChainConfig `json:"config"`
	Head       common.Hash         `json:"head"`
}

func (self *ProtocolManager) NodeInfo() *NodeInfo {
	currentBlock := self.blockchain.CurrentBlock()
	return &NodeInfo{
		Network:    self.networkId,
		Difficulty: self.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64()),
		Genesis:    self.blockchain.Genesis().Hash(),
		Config:     self.blockchain.Config(),
		Head:       currentBlock.Hash(),
	}
}

func (self *ProtocolManager) FindPeers(targets map[common.Address]bool) map[common.Address]consensus.Peer {
	m := make(map[common.Address]consensus.Peer)
	for _, p := range self.peers.Peers() {
		pubKey, err := p.ID().Pubkey()
		if err != nil {
			continue
		}
		addr := crypto.PubkeyToAddress(*pubKey)
		if targets[addr] {
			m[addr] = p
		}
	}
	return m
}
