package main

import (
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"sync"

	"github.com/nio-net/crypto"
	dbm "github.com/nio-net/database"
	"github.com/nio-net/nio/chain/accounts"
	"github.com/nio-net/nio/chain/consensus"
	"github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	"github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/core"
	"github.com/nio-net/nio/chain/core/rawdb"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/neatcli"
	"github.com/nio-net/nio/neatptc"
	"github.com/nio-net/nio/network/node"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/utils"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

type ChainManager struct {
	ctx *cli.Context

	mainChain     *Chain
	mainQuit      <-chan struct{}
	mainStartDone chan struct{}

	createSideChainLock sync.Mutex
	sideChains          map[string]*Chain
	sideQuits           map[string]<-chan struct{}

	stop chan struct{}

	server *utils.NeatChainP2PServer
	cch    *CrossChainHelper
}

var chainMgr *ChainManager
var once sync.Once

func GetCMInstance(ctx *cli.Context) *ChainManager {

	once.Do(func() {
		chainMgr = &ChainManager{ctx: ctx}
		chainMgr.stop = make(chan struct{})
		chainMgr.sideChains = make(map[string]*Chain)
		chainMgr.sideQuits = make(map[string]<-chan struct{})
		chainMgr.cch = &CrossChainHelper{}
	})
	return chainMgr
}

func (cm *ChainManager) GetNodeID() string {
	return cm.server.Server().NodeInfo().ID
}

func (cm *ChainManager) InitP2P() {
	cm.server = utils.NewP2PServer(cm.ctx)
}

func (cm *ChainManager) LoadMainChain() error {

	chainId := MainChain
	if cm.ctx.GlobalBool(utils.TestnetFlag.Name) {
		chainId = TestnetChain
	}
	cm.mainChain = LoadMainChain(cm.ctx, chainId)
	if cm.mainChain == nil {
		return errors.New("Load main chain failed")
	}

	return nil
}

func (cm *ChainManager) LoadChains(sideIds []string) error {

	sideChainIds := core.GetSideChainIds(cm.cch.chainInfoDB)

	readyToLoadChains := make(map[string]bool)

	for _, chainId := range sideChainIds {

		ci := core.GetChainInfo(cm.cch.chainInfoDB, chainId)

		if ci.Epoch != nil && cm.checkCoinbaseInSideChain(ci.Epoch) {
			readyToLoadChains[chainId] = true
		}
	}

	for _, requestId := range sideIds {
		if requestId == "" {

			continue
		}

		if _, present := readyToLoadChains[requestId]; present {

			continue
		} else {

			readyToLoadChains[requestId] = false
		}
	}

	for chainId := range readyToLoadChains {
		chain := LoadSideChain(cm.ctx, chainId)
		if chain == nil {
			log.Errorf("Load side chain: %s Failed.", chainId)
			continue
		}

		cm.sideChains[chainId] = chain
		log.Infof("Load side chain: %s Success!", chainId)
	}
	return nil
}

func (cm *ChainManager) InitCrossChainHelper() {
	cm.cch.chainInfoDB = dbm.NewDB("chaininfo",
		cm.mainChain.Config.GetString("db_backend"),
		cm.ctx.GlobalString(utils.DataDirFlag.Name))
	cm.cch.localTX3CacheDB, _ = rawdb.NewLevelDBDatabase(path.Join(cm.ctx.GlobalString(utils.DataDirFlag.Name), "tx3cache"), 0, 0, "neatio/db/tx3/")

	chainId := MainChain
	if cm.ctx.GlobalBool(utils.TestnetFlag.Name) {
		chainId = TestnetChain
	}
	cm.cch.mainChainId = chainId

	if cm.ctx.GlobalBool(utils.RPCEnabledFlag.Name) {
		host := "127.0.0.1"
		port := cm.ctx.GlobalInt(utils.RPCPortFlag.Name)
		url := net.JoinHostPort(host, strconv.Itoa(port))
		url = "http://" + url + "/" + chainId
		client, err := neatcli.Dial(url)
		if err != nil {
			log.Errorf("can't connect to %s, err: %v, exit", url, err)
			os.Exit(0)
		}
		cm.cch.client = client
	}
}

func (cm *ChainManager) StartP2PServer() error {
	srv := cm.server.Server()

	srv.Protocols = append(srv.Protocols, cm.mainChain.NeatNode.GatherProtocols()...)

	return srv.Start()
}

func (cm *ChainManager) StartMainChain() error {

	cm.mainStartDone = make(chan struct{})

	cm.mainChain.NeatNode.SetP2PServer(cm.server.Server())

	if address, ok := cm.getNodeValidator(cm.mainChain.NeatNode); ok {
		cm.server.AddLocalValidator(cm.mainChain.Id, address)
	}

	err := StartChain(cm.ctx, cm.mainChain, cm.mainStartDone)

	<-cm.mainStartDone
	cm.mainQuit = cm.mainChain.NeatNode.StopChan()

	return err
}

func (cm *ChainManager) StartChains() error {

	for _, chain := range cm.sideChains {

		srv := cm.server.Server()
		sideProtocols := chain.NeatNode.GatherProtocols()

		srv.Protocols = append(srv.Protocols, sideProtocols...)

		srv.AddChildProtocolCaps(sideProtocols)

		chain.NeatNode.SetP2PServer(srv)

		if address, ok := cm.getNodeValidator(chain.NeatNode); ok {
			cm.server.AddLocalValidator(chain.Id, address)
		}

		startDone := make(chan struct{})
		StartChain(cm.ctx, chain, startDone)
		<-startDone

		cm.sideQuits[chain.Id] = chain.NeatNode.StopChan()

		cm.server.BroadcastNewSideChainMsg(chain.Id)
	}

	return nil
}

func (cm *ChainManager) StartRPC() error {

	err := utils.StartRPC(cm.ctx)
	if err != nil {
		return err
	} else {
		if utils.IsHTTPRunning() {
			if h, err := cm.mainChain.NeatNode.GetHTTPHandler(); err == nil {
				utils.HookupHTTP(cm.mainChain.Id, h)
			} else {
				log.Errorf("Load Main Chain RPC HTTP handler failed: %v", err)
			}
			for _, chain := range cm.sideChains {
				if h, err := chain.NeatNode.GetHTTPHandler(); err == nil {
					utils.HookupHTTP(chain.Id, h)
				} else {
					log.Errorf("Load Side Chain RPC HTTP handler failed: %v", err)
				}
			}
		}

		if utils.IsWSRunning() {
			if h, err := cm.mainChain.NeatNode.GetWSHandler(); err == nil {
				utils.HookupWS(cm.mainChain.Id, h)
			} else {
				log.Errorf("Load Main Chain RPC WS handler failed: %v", err)
			}
			for _, chain := range cm.sideChains {
				if h, err := chain.NeatNode.GetWSHandler(); err == nil {
					utils.HookupWS(chain.Id, h)
				} else {
					log.Errorf("Load Side Chain RPC WS handler failed: %v", err)
				}
			}
		}
	}

	return nil
}

func (cm *ChainManager) StartInspectEvent() {

	createSideChainCh := make(chan core.CreateSideChainEvent, 10)
	createSideChainSub := MustGetNeatChainFromNode(cm.mainChain.NeatNode).BlockChain().SubscribeCreateSideChainEvent(createSideChainCh)

	go func() {
		defer createSideChainSub.Unsubscribe()

		for {
			select {
			case event := <-createSideChainCh:
				log.Infof("CreateSideChainEvent received: %v", event)

				go func() {
					cm.createSideChainLock.Lock()
					defer cm.createSideChainLock.Unlock()

					cm.LoadSideChainInRT(event.ChainId)
				}()
			case <-createSideChainSub.Err():
				return
			}
		}
	}()
}

func (cm *ChainManager) LoadSideChainInRT(chainId string) {

	cci := core.GetPendingSideChainData(cm.cch.chainInfoDB, chainId)
	if cci == nil {
		log.Errorf("side chain: %s does not exist, can't load", chainId)
		return
	}

	validators := make([]types.GenesisValidator, 0, len(cci.JoinedValidators))

	validator := false

	var neatio *neatptc.NeatIO
	cm.mainChain.NeatNode.Service(&neatio)

	var localEtherbase common.Address
	if neatcon, ok := neatio.Engine().(consensus.NeatCon); ok {
		localEtherbase = neatcon.PrivateValidator()
	}

	for _, v := range cci.JoinedValidators {
		if v.Address == localEtherbase {
			validator = true
		}

		if pubkey, ok := v.PubKey.(*crypto.BLSPubKey); ok {
			v.PubKey = *pubkey
		}

		validators = append(validators, types.GenesisValidator{
			EthAccount: v.Address,
			PubKey:     v.PubKey,
			Amount:     v.DepositAmount,
		})
	}

	defer writeGenesisIntoChainInfoDB(cm.cch.chainInfoDB, chainId, validators)

	if !validator {
		log.Warnf("You are not in the validators of side chain %v, no need to start the side chain", chainId)

		cm.formalizeSideChain(chainId, *cci, nil)
		return
	}

	if _, ok := cm.sideChains[chainId]; ok {
		log.Infof("Side Chain [%v] has been already loaded.", chainId)
		return
	}

	var keyJson []byte
	wallet, walletErr := cm.mainChain.NeatNode.AccountManager().Find(accounts.Account{Address: localEtherbase})
	if walletErr == nil {
		var readKeyErr error
		keyJson, readKeyErr = ioutil.ReadFile(wallet.URL().Path)
		if readKeyErr != nil {
			log.Errorf("Failed to Read the KeyStore %v, Error: %v", localEtherbase, readKeyErr)
		}
	}

	privValidatorFile := cm.mainChain.Config.GetString("priv_validator_file")
	self := types.LoadPrivValidator(privValidatorFile)

	err := CreateSideChain(cm.ctx, chainId, *self, keyJson, validators)
	if err != nil {
		log.Errorf("Create Side Chain %v failed! %v", chainId, err)
		return
	}

	chain := LoadSideChain(cm.ctx, chainId)
	if chain == nil {
		log.Errorf("Side Chain %v load failed!", chainId)
		return
	}

	srv := cm.server.Server()
	sideProtocols := chain.NeatNode.GatherProtocols()

	srv.Protocols = append(srv.Protocols, sideProtocols...)

	srv.AddChildProtocolCaps(sideProtocols)

	chain.NeatNode.SetP2PServer(srv)

	if address, ok := cm.getNodeValidator(chain.NeatNode); ok {
		srv.AddLocalValidator(chain.Id, address)
	}

	startDone := make(chan struct{})
	err = StartChain(cm.ctx, chain, startDone)
	<-startDone
	if err != nil {
		return
	}

	cm.sideQuits[chain.Id] = chain.NeatNode.StopChan()

	var sideEthereum *neatptc.NeatIO
	chain.NeatNode.Service(&sideEthereum)
	firstEpoch := sideEthereum.Engine().(consensus.NeatCon).GetEpoch()

	cm.formalizeSideChain(chainId, *cci, firstEpoch)

	cm.sideChains[chainId] = chain

	go cm.server.BroadcastNewSideChainMsg(chainId)

	if utils.IsHTTPRunning() {
		if h, err := chain.NeatNode.GetHTTPHandler(); err == nil {
			utils.HookupHTTP(chain.Id, h)
		} else {
			log.Errorf("Unable Hook up Side Chain (%v) RPC HTTP Handler: %v", chainId, err)
		}
	}
	if utils.IsWSRunning() {
		if h, err := chain.NeatNode.GetWSHandler(); err == nil {
			utils.HookupWS(chain.Id, h)
		} else {
			log.Errorf("Unable Hook up Side Chain (%v) RPC WS Handler: %v", chainId, err)
		}
	}

}

func (cm *ChainManager) formalizeSideChain(chainId string, cci core.CoreChainInfo, ep *epoch.Epoch) {

	core.DeletePendingSideChainData(cm.cch.chainInfoDB, chainId)

	core.SaveChainInfo(cm.cch.chainInfoDB, &core.ChainInfo{CoreChainInfo: cci, Epoch: ep})
}

func (cm *ChainManager) checkCoinbaseInSideChain(sideEpoch *epoch.Epoch) bool {
	var neatio *neatptc.NeatIO
	cm.mainChain.NeatNode.Service(&neatio)

	var localEtherbase common.Address
	if neatcon, ok := neatio.Engine().(consensus.NeatCon); ok {
		localEtherbase = neatcon.PrivateValidator()
	}

	return sideEpoch.Validators.HasAddress(localEtherbase[:])
}

func (cm *ChainManager) StopChain() {
	go func() {
		mainChainError := cm.mainChain.NeatNode.Close()
		if mainChainError != nil {
			log.Error("Error when closing main chain", "err", mainChainError)
		} else {
			log.Info("Main Chain Closed")
		}
	}()
	for _, side := range cm.sideChains {
		go func() {
			sideChainError := side.NeatNode.Close()
			if sideChainError != nil {
				log.Error("Error when closing side chain", "side id", side.Id, "err", sideChainError)
			}
		}()
	}
}

func (cm *ChainManager) WaitChainsStop() {
	<-cm.mainQuit
	for _, quit := range cm.sideQuits {
		<-quit
	}
}

func (cm *ChainManager) Stop() {
	utils.StopRPC()
	cm.server.Stop()
	cm.cch.localTX3CacheDB.Close()
	cm.cch.chainInfoDB.Close()

	close(cm.stop)
}

func (cm *ChainManager) Wait() {
	<-cm.stop
}

func (cm *ChainManager) getNodeValidator(neatNode *node.Node) (common.Address, bool) {

	var neatio *neatptc.NeatIO
	neatNode.Service(&neatio)

	var coinbase common.Address
	ntc := neatio.Engine()
	epoch := ntc.GetEpoch()
	coinbase = ntc.PrivateValidator()
	log.Debugf("getNodeValidator() coinbase is :%v", coinbase)
	return coinbase, epoch.Validators.HasAddress(coinbase[:])
}

func writeGenesisIntoChainInfoDB(db dbm.DB, sideChainId string, validators []types.GenesisValidator) {
	ethByte, _ := generateETHGenesis(sideChainId, validators)
	ntcByte, _ := generateNTCGenesis(sideChainId, validators)
	core.SaveChainGenesis(db, sideChainId, ethByte, ntcByte)
}
