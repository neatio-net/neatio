package neatptc

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/neatlab/neatio/neatptc/downloader"
	"github.com/neatlab/neatio/network/p2p"
	"github.com/neatlab/neatio/network/p2p/discover"
)

func TestFastSyncDisabling(t *testing.T) {

	pmEmpty, _ := newTestProtocolManagerMust(t, downloader.FastSync, 0, nil, nil)
	if atomic.LoadUint32(&pmEmpty.fastSync) == 0 {
		t.Fatalf("fast sync disabled on pristine blockchain")
	}

	pmFull, _ := newTestProtocolManagerMust(t, downloader.FastSync, 1024, nil, nil)
	if atomic.LoadUint32(&pmFull.fastSync) == 1 {
		t.Fatalf("fast sync not disabled on non-empty blockchain")
	}

	io1, io2 := p2p.MsgPipe()

	go pmFull.handle(pmFull.newPeer(63, p2p.NewPeer(discover.NodeID{}, "empty", nil), io2))
	go pmEmpty.handle(pmEmpty.newPeer(63, p2p.NewPeer(discover.NodeID{}, "full", nil), io1))

	time.Sleep(250 * time.Millisecond)
	pmEmpty.synchronise(pmEmpty.peers.BestPeer())

	if atomic.LoadUint32(&pmEmpty.fastSync) == 1 {
		t.Fatalf("fast sync not disabled after successful synchronisation")
	}
}
