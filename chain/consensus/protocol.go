package consensus

import (
	"math/big"

	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/utilities/common"
)

const (
	Eth62 = 62
	Eth63 = 63
)

var (
	EthProtocol = Protocol{
		Name:     "eth",
		Versions: []uint{Eth62, Eth63},
		Lengths:  []uint64{17, 8},
	}
)

type Protocol struct {
	Name string

	Versions []uint

	Lengths []uint64
}

type Broadcaster interface {
	Enqueue(id string, block *types.Block)

	FindPeers(map[common.Address]bool) map[common.Address]Peer

	BroadcastBlock(block *types.Block, propagate bool)

	BroadcastMessage(msgcode uint64, data interface{})

	TryFixBadPreimages()
}

type Peer interface {
	Send(msgcode uint64, data interface{}) error

	SendNewBlock(block *types.Block, td *big.Int) error

	GetPeerState() PeerState

	GetKey() string

	GetConsensusKey() string

	SetPeerState(ps PeerState)
}

type PeerState interface {
	GetHeight() uint64
	Disconnect()
}
