package neatpos

import (
	"errors"

	"github.com/neatlab/neatio/consensus"
	ncTypes "github.com/neatlab/neatio/consensus/neatpos/types"
	"github.com/neatlab/neatio/core/types"
	"github.com/neatlab/neatio/log"
	"github.com/neatlab/neatio/params"
)

var (
	errDecodeFailed = errors.New("fail to decode neatpos message")
)

func (sb *backend) Protocol() consensus.Protocol {

	sb.logger.Info("NeatPoS backend protocol")

	var protocolName string
	if sb.chainConfig.NeatChainId == params.MainnetChainConfig.NeatChainId || sb.chainConfig.NeatChainId == params.TestnetChainConfig.NeatChainId {
		protocolName = "neatio"
	} else {
		protocolName = "neatio_" + sb.chainConfig.NeatChainId
	}

	return consensus.Protocol{
		Name:     protocolName,
		Versions: []uint{64},
		Lengths:  []uint64{64},
	}
}

func (sb *backend) HandleMsg(chID uint64, src consensus.Peer, msgBytes []byte) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	sb.core.consensusReactor.Receive(chID, src, msgBytes)

	return false, nil
}

func (sb *backend) SetBroadcaster(broadcaster consensus.Broadcaster) {

	sb.logger.Infof("NeatPoS SetBroadcaster: %p", broadcaster)
	sb.broadcaster = broadcaster
}

func (sb *backend) GetBroadcaster() consensus.Broadcaster {

	sb.logger.Infof("NeatPoS GetBroadcaster: %p", sb.broadcaster)
	return sb.broadcaster
}

func (sb *backend) NewChainHead(block *types.Block) error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return ErrStoppedEngine
	}
	go ncTypes.FireEventFinalCommitted(sb.core.EventSwitch(), ncTypes.EventDataFinalCommitted{block.NumberU64()})
	return nil
}

func (sb *backend) GetLogger() log.Logger {
	return sb.logger
}

func (sb *backend) AddPeer(src consensus.Peer) {

	sb.core.consensusReactor.AddPeer(src)
	sb.logger.Debug("Peer successful added into Consensus Reactor")

}

func (sb *backend) RemovePeer(src consensus.Peer) {
	sb.core.consensusReactor.RemovePeer(src, nil)
}
