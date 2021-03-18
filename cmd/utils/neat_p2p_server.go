package utils

import (
	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/node"
	"github.com/neatlab/neatio/p2p"
	"gopkg.in/urfave/cli.v1"
)

type NeatChainP2PServer struct {
	serverConfig p2p.Config
	server       *p2p.Server
}

func NewP2PServer(ctx *cli.Context) *NeatChainP2PServer {

	// Load Default P2P config
	config := &node.Config{
		GeneralDataDir: MakeDataDir(ctx),
		DataDir:        MakeDataDir(ctx), // Just for pass the check, P2P always use GeneralDataDir
		P2P:            node.DefaultConfig.P2P,
	}

	// Setup the config from context
	SetP2PConfig(ctx, &config.P2P)

	serverConfig := config.P2P
	serverConfig.PrivateKey = config.NodeKey()
	serverConfig.Name = config.NodeName()
	serverConfig.EnableMsgEvents = true

	if serverConfig.StaticNodes == nil {
		serverConfig.StaticNodes = config.StaticNodes()
	}
	if serverConfig.TrustedNodes == nil {
		serverConfig.TrustedNodes = config.TrustedNodes()
	}
	if serverConfig.NodeDatabase == "" {
		serverConfig.NodeDatabase = config.NodeDB()
	}
	serverConfig.LocalValidators = make([]p2p.P2PValidator, 0)
	serverConfig.Validators = make(map[p2p.P2PValidator]*p2p.P2PValidatorNodeInfo)

	running := &p2p.Server{Config: serverConfig}
	//log.Info("Create peer-to-peer node", "instance", serverConfig.Name)

	return &NeatChainP2PServer{
		serverConfig: serverConfig,
		server:       running,
	}
}

func (srv *NeatChainP2PServer) Server() *p2p.Server {
	return srv.server
}

func (srv *NeatChainP2PServer) Stop() {
	srv.server.Stop()
}

func (srv *NeatChainP2PServer) BroadcastNewSideChainMsg(childId string) {
	srv.server.BroadcastMsg(p2p.BroadcastNewSideChainMsg, childId)
}

func (srv *NeatChainP2PServer) AddLocalValidator(chainId string, address common.Address) {
	srv.server.AddLocalValidator(chainId, address)
}

func (srv *NeatChainP2PServer) RemoveLocalValidator(chainId string, address common.Address) {
	srv.server.RemoveLocalValidator(chainId, address)
}
