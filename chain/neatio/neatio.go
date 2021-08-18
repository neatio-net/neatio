package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/neatlab/neatio/chain/consensus/neatcon/consensus"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/internal/debug"
	"github.com/neatlab/neatio/utilities/utils"
	"gopkg.in/urfave/cli.v1"
)

func neatchainCmd(ctx *cli.Context) error {

	if ctx == nil {
		log.Error("oh, ctx is null, how neatio works?")
		return nil
	}

	log.Info("NEAT | Blazing FAST, ultra SECURE and ECO friendly payment solution.")

	chainMgr := GetCMInstance(ctx)

	requestSideChain := strings.Split(ctx.GlobalString(utils.SideChainFlag.Name), ",")

	chainMgr.InitP2P()

	err := chainMgr.LoadMainChain()
	if err != nil {
		log.Errorf("Load Main Chain failed. %v", err)
		return nil
	}

	chainMgr.InitCrossChainHelper()

	err = chainMgr.StartP2PServer()
	if err != nil {
		log.Errorf("Start P2P Server failed. %v", err)
		return err
	}
	consensus.NodeID = chainMgr.GetNodeID()[0:16]

	err = chainMgr.StartMainChain()

	err = chainMgr.LoadChains(requestSideChain)
	if err != nil {
		log.Errorf("Load Side Chains failed. %v", err)
		return err
	}

	err = chainMgr.StartChains()
	if err != nil {
		log.Error("start chains failed")
		return err
	}

	err = chainMgr.StartRPC()
	if err != nil {
		log.Error("start NEAT RPC failed")
		return err
	}

	chainMgr.StartInspectEvent()

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("Got interrupt, shutting down...")

		chainMgr.StopChain()
		chainMgr.WaitChainsStop()
		chainMgr.Stop()

		for i := 10; i > 0; i-- {
			<-sigc
			if i > 1 {
				log.Info(fmt.Sprintf("Already shutting down, interrupt %d more times for panic.", i-1))
			}
		}
		debug.Exit()
		debug.LoudPanic("boom")
	}()

	chainMgr.Wait()

	return nil
}
