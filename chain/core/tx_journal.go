package core

import (
	"errors"
	"io"
	"os"

	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/log"
	neatAbi "github.com/nio-net/nio/neatabi/abi"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/rlp"
)

var errNoActiveJournal = errors.New("no active journal")

type devNull struct{}

func (*devNull) Write(p []byte) (n int, err error) { return len(p), nil }
func (*devNull) Close() error                      { return nil }

type txJournal struct {
	path   string
	writer io.WriteCloser
}

func newTxJournal(path string) *txJournal {
	return &txJournal{
		path: path,
	}
}

func (journal *txJournal) load(add func(*types.Transaction) error) error {
	if _, err := os.Stat(journal.path); os.IsNotExist(err) {
		return nil
	}
	input, err := os.Open(journal.path)
	if err != nil {
		return err
	}
	defer input.Close()

	journal.writer = new(devNull)
	defer func() { journal.writer = nil }()

	stream := rlp.NewStream(input, 0)
	total, dropped := 0, 0

	var failure error
	for {
		tx := new(types.Transaction)
		if err = stream.Decode(tx); err != nil {
			if err != io.EOF {
				failure = err
			}
			break
		}
		if neatAbi.IsNeatChainContractAddr(tx.To()) {
			continue
		}
		total++
		if err = add(tx); err != nil {
			log.Debug("Failed to add journaled transaction", "err", err)
			dropped++
			continue
		}
	}

	return failure
}

func (journal *txJournal) insert(tx *types.Transaction) error {
	if journal.writer == nil {
		return errNoActiveJournal
	}
	if err := rlp.Encode(journal.writer, tx); err != nil {
		return err
	}
	return nil
}

func (journal *txJournal) rotate(all map[common.Address]types.Transactions) error {
	if journal.writer != nil {
		if err := journal.writer.Close(); err != nil {
			return err
		}
		journal.writer = nil
	}
	replacement, err := os.OpenFile(journal.path+".new", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	journaled := 0
	for _, txs := range all {
		for _, tx := range txs {
			if err = rlp.Encode(replacement, tx); err != nil {
				replacement.Close()
				return err
			}
		}
		journaled += len(txs)
	}
	replacement.Close()

	if err = os.Rename(journal.path+".new", journal.path); err != nil {
		return err
	}
	sink, err := os.OpenFile(journal.path, os.O_WRONLY|os.O_APPEND, 0755)
	if err != nil {
		return err
	}
	journal.writer = sink

	return nil
}

func (journal *txJournal) close() error {
	var err error

	if journal.writer != nil {
		err = journal.writer.Close()
		journal.writer = nil
	}
	return err
}
