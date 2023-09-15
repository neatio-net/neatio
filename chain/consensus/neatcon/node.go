package neatcon

import (
	"io/ioutil"
	"os"
	"strings"

	cmn "github.com/nio-net/common"
	cfg "github.com/nio-net/config"
	dbm "github.com/nio-net/database"
	"github.com/nio-net/nio/chain/consensus/neatcon/consensus"
	"github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	"github.com/nio-net/nio/chain/consensus/neatcon/types"
	"github.com/nio-net/nio/chain/core"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/params"
)

type Node struct {
	cmn.BaseService

	privValidator *types.PrivValidator

	epochDB dbm.DB

	evsw             types.EventSwitch
	consensusState   *consensus.ConsensusState
	consensusReactor *consensus.ConsensusReactor

	cch    core.CrossChainHelper
	logger log.Logger
}

func NewNodeNotStart(backend *backend, config cfg.Config, chainConfig *params.ChainConfig, cch core.CrossChainHelper, genDoc *types.GenesisDoc) *Node {
	var privValidator *types.PrivValidator
	privValidatorFile := config.GetString("priv_validator_file")
	if _, err := os.Stat(privValidatorFile); err == nil {
		privValidator = types.LoadPrivValidator(privValidatorFile)
	}

	epochDB := dbm.NewDB("epoch", config.GetString("db_backend"), config.GetString("db_dir"))
	ep := epoch.InitEpoch(epochDB, genDoc, backend.logger)

	if privValidator != nil && ep.Validators.HasAddress(privValidator.Address[:]) {
		backend.shouldStart = true
	} else {
		backend.shouldStart = false
	}

	consensusState := consensus.NewConsensusState(backend, config, chainConfig, cch, ep)
	if privValidator != nil {
		consensusState.SetPrivValidator(privValidator)
	}
	consensusReactor := consensus.NewConsensusReactor(consensusState)

	eventSwitch := types.NewEventSwitch()
	SetEventSwitch(eventSwitch, consensusReactor)

	node := &Node{
		privValidator: privValidator,

		epochDB: epochDB,

		evsw: eventSwitch,

		cch: cch,

		consensusState:   consensusState,
		consensusReactor: consensusReactor,

		logger: backend.logger,
	}
	node.BaseService = *cmn.NewBaseService(backend.logger, "Node", node)

	return node
}

func (n *Node) OnStart() error {

	n.logger.Info("(n *Node) OnStart()")

	if n.privValidator == nil {
		return ErrNoPrivValidator
	}

	_, err := n.evsw.Start()
	if err != nil {
		n.logger.Errorf("Failed to start switch: %v", err)
		return err
	}

	_, err = n.consensusReactor.Start()
	if err != nil {
		n.logger.Errorf("Failed to start Consensus Reactor. Error: %v", err)
		return err
	}

	n.consensusReactor.AfterStart()

	return nil
}

func (n *Node) OnStop() {
	n.logger.Info("(n *Node) OnStop() called")
	n.BaseService.OnStop()

	n.evsw.Stop()
	n.consensusReactor.Stop()
}

func (n *Node) RunForever() {
	cmn.TrapSignal(func() {
		n.Stop()
	})
}

func SetEventSwitch(evsw types.EventSwitch, eventables ...types.Eventable) {
	for _, e := range eventables {
		e.SetEventSwitch(evsw)
	}
}

func (n *Node) ConsensusState() *consensus.ConsensusState {
	return n.consensusState
}

func (n *Node) ConsensusReactor() *consensus.ConsensusReactor {
	return n.consensusReactor
}

func (n *Node) EventSwitch() types.EventSwitch {
	return n.evsw
}

func (n *Node) PrivValidator() *types.PrivValidator {
	return n.privValidator
}

func ProtocolAndAddress(listenAddr string) (string, string) {
	protocol, address := "tcp", listenAddr
	parts := strings.SplitN(address, "://", 2)
	if len(parts) == 2 {
		protocol, address = parts[0], parts[1]
	}
	return protocol, address
}

func MakeNeatConNode(backend *backend, config cfg.Config, chainConfig *params.ChainConfig, cch core.CrossChainHelper) *Node {

	var genDoc *types.GenesisDoc
	genDocFile := config.GetString("genesis_file")

	if !cmn.FileExists(genDocFile) {
		if chainConfig.NeatChainId == params.MainnetChainConfig.NeatChainId {
			genDoc, _ = types.GenesisDocFromJSON([]byte(types.MainnetGenesisJSON))
		} else if chainConfig.NeatChainId == params.TestnetChainConfig.NeatChainId {
			genDoc, _ = types.GenesisDocFromJSON([]byte(types.TestnetGenesisJSON))
		} else {
			return nil
		}
	} else {
		genDoc = readGenesisFromFile(genDocFile)
	}
	config.Set("chain_id", genDoc.ChainID)

	return NewNodeNotStart(backend, config, chainConfig, cch, genDoc)
}

func readGenesisFromFile(genDocFile string) *types.GenesisDoc {
	jsonBlob, err := ioutil.ReadFile(genDocFile)
	if err != nil {
		cmn.Exit(cmn.Fmt("Couldn't read GenesisDoc file: %v", err))
	}
	genDoc, err := types.GenesisDocFromJSON(jsonBlob)
	if err != nil {
		cmn.PanicSanity(cmn.Fmt("Genesis doc parse json error: %v", err))
	}
	if genDoc.ChainID == "" {
		cmn.PanicSanity(cmn.Fmt("Genesis doc %v must include non-empty chain_id", genDocFile))
	}
	return genDoc
}
