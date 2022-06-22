package utils

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/neatlab/neatio/chain/consensus"
	neatptc "github.com/neatlab/neatio/neatptc"
	"gopkg.in/urfave/cli.v1"

	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/chain/core/rawdb"
	"github.com/neatio-network/neatio/chain/core/types"
	"github.com/neatio-network/neatio/chain/log"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/network/node"
	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/crypto"
	"github.com/neatlab/neatio/utilities/rlp"
)

const (
	importBatchSize = 2500
)

func Fatalf(format string, args ...interface{}) {
	w := io.MultiWriter(os.Stdout, os.Stderr)
	if runtime.GOOS == "windows" {

		w = os.Stdout
	} else {
		outf, _ := os.Stdout.Stat()
		errf, _ := os.Stderr.Stat()
		if outf != nil && errf != nil && os.SameFile(outf, errf) {
			w = os.Stderr
		}
	}
	fmt.Fprintf(w, "Fatal: "+format+"\n", args...)
	os.Exit(1)
}

func StartNode(ctx *cli.Context, stack *node.Node) error {
	if err := stack.Start1(); err != nil {
		Fatalf("Error starting protocol stack: %v", err)
	}

	mining := false
	var neatio *neatptc.NeatIO
	if err := stack.Service(&neatio); err == nil {
		if neatcon, ok := neatio.Engine().(consensus.NeatCon); ok {
			mining = neatcon.ShouldStart()
			if mining {
				stack.GetLogger().Info("NeatCon Consensus Engine will be start shortly")
			}
		}
	}

	if mining || ctx.GlobalBool(DeveloperFlag.Name) {
		stack.GetLogger().Info("Mine will be start shortly")

		var neatio *neatptc.NeatIO
		if err := stack.Service(&neatio); err != nil {
			Fatalf("NEAT Blockchain service not running: %v", err)
		}

		if threads := ctx.GlobalInt(MinerThreadsFlag.Name); threads > 0 {
			type threaded interface {
				SetThreads(threads int)
			}
			if th, ok := neatio.Engine().(threaded); ok {
				th.SetThreads(threads)
			}
		}

		neatio.TxPool().SetGasPrice(GlobalBig(ctx, MinerGasPriceFlag.Name))
		if err := neatio.StartMining(true); err != nil {
			Fatalf("Failed to start mining: %v", err)
		}
	}

	return nil
}

func ImportChain(chain *core.BlockChain, fn string) error {

	interrupt := make(chan os.Signal, 1)
	stop := make(chan struct{})
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	defer close(interrupt)
	go func() {
		if _, ok := <-interrupt; ok {
			log.Info("Interrupted during import, stopping at next batch")
		}
		close(stop)
	}()
	checkInterrupt := func() bool {
		select {
		case <-stop:
			return true
		default:
			return false
		}
	}

	log.Info("Importing blockchain", "file", fn)
	fh, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer fh.Close()

	var reader io.Reader = fh
	if strings.HasSuffix(fn, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return err
		}
	}
	stream := rlp.NewStream(reader, 0)

	blocks := make(types.Blocks, importBatchSize)
	n := 0
	for batch := 0; ; batch++ {

		if checkInterrupt() {
			return fmt.Errorf("interrupted")
		}
		i := 0
		for ; i < importBatchSize; i++ {
			var b types.Block
			if err := stream.Decode(&b); err == io.EOF {
				break
			} else if err != nil {
				return fmt.Errorf("at block %d: %v", n, err)
			}

			if b.NumberU64() == 0 {
				i--
				continue
			}
			blocks[i] = &b
			n++
		}
		if i == 0 {
			break
		}

		if checkInterrupt() {
			return fmt.Errorf("interrupted")
		}
		missing := missingBlocks(chain, blocks[:i])
		if len(missing) == 0 {
			log.Info("Skipping batch as all blocks present", "batch", batch, "first", blocks[0].Hash(), "last", blocks[i-1].Hash())
			continue
		}
		if _, err := chain.InsertChain(missing); err != nil {
			return fmt.Errorf("invalid block %d: %v", n, err)
		}
	}
	return nil
}

func missingBlocks(chain *core.BlockChain, blocks []*types.Block) []*types.Block {
	head := chain.CurrentBlock()
	for i, block := range blocks {

		if head.NumberU64() > block.NumberU64() {
			if !chain.HasBlock(block.Hash(), block.NumberU64()) {
				return blocks[i:]
			}
			continue
		}

		if !chain.HasBlockAndState(block.Hash(), block.NumberU64()) {
			return blocks[i:]
		}
	}
	return nil
}

func ExportChain(blockchain *core.BlockChain, fn string) error {
	log.Info("Exporting blockchain", "file", fn)
	fh, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	defer fh.Close()

	var writer io.Writer = fh
	if strings.HasSuffix(fn, ".gz") {
		writer = gzip.NewWriter(writer)
		defer writer.(*gzip.Writer).Close()
	}

	if err := blockchain.Export(writer); err != nil {
		return err
	}
	log.Info("Exported blockchain", "file", fn)

	return nil
}

func ExportAppendChain(blockchain *core.BlockChain, fn string, first uint64, last uint64) error {
	log.Info("Exporting blockchain", "file", fn)

	fh, err := os.OpenFile(fn, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer fh.Close()

	var writer io.Writer = fh
	if strings.HasSuffix(fn, ".gz") {
		writer = gzip.NewWriter(writer)
		defer writer.(*gzip.Writer).Close()
	}

	if err := blockchain.ExportN(writer, first, last); err != nil {
		return err
	}
	log.Info("Exported blockchain to", "file", fn)
	return nil
}

func ImportPreimages(db neatdb.Database, fn string) error {
	log.Info("Importing preimages", "file", fn)

	fh, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer fh.Close()

	var reader io.Reader = fh
	if strings.HasSuffix(fn, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return err
		}
	}
	stream := rlp.NewStream(reader, 0)

	preimages := make(map[common.Hash][]byte)

	for {

		var blob []byte

		if err := stream.Decode(&blob); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		preimages[crypto.Keccak256Hash(blob)] = common.CopyBytes(blob)
		if len(preimages) > 1024 {
			rawdb.WritePreimages(db, preimages)
			preimages = make(map[common.Hash][]byte)
		}
	}

	if len(preimages) > 0 {
		rawdb.WritePreimages(db, preimages)
	}
	return nil
}

func ExportPreimages(db neatdb.Database, fn string) error {
	log.Info("Exporting preimages", "file", fn)

	fh, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	defer fh.Close()

	var writer io.Writer = fh
	if strings.HasSuffix(fn, ".gz") {
		writer = gzip.NewWriter(writer)
		defer writer.(*gzip.Writer).Close()
	}

	it := db.NewIteratorWithPrefix([]byte("secure-key-"))
	defer it.Release()

	for it.Next() {
		if err := rlp.Encode(writer, it.Value()); err != nil {
			return err
		}
	}
	log.Info("Exported preimages", "file", fn)
	return nil
}
