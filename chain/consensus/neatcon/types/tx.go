package types

import (
	"bytes"
	"errors"

	"github.com/nio-net/merkle"
)

type Tx []byte

func (tx Tx) Hash() []byte {
	return merkle.SimpleHashFromBinary(tx)
}

type Txs []Tx

func (txs Txs) Hash() []byte {

	switch len(txs) {
	case 0:
		return nil
	case 1:
		return txs[0].Hash()
	default:
		left := Txs(txs[:(len(txs)+1)/2]).Hash()
		right := Txs(txs[(len(txs)+1)/2:]).Hash()
		return merkle.SimpleHashFromTwoHashes(left, right)
	}
}

func (txs Txs) Index(tx Tx) int {
	for i := range txs {
		if bytes.Equal(txs[i], tx) {
			return i
		}
	}
	return -1
}

func (txs Txs) IndexByHash(hash []byte) int {
	for i := range txs {
		if bytes.Equal(txs[i].Hash(), hash) {
			return i
		}
	}
	return -1
}

func (txs Txs) Proof(i int) TxProof {
	l := len(txs)
	hashables := make([]merkle.Hashable, l)
	for i := 0; i < l; i++ {
		hashables[i] = txs[i]
	}
	root, proofs := merkle.SimpleProofsFromHashables(hashables)

	return TxProof{
		Index:    i,
		Total:    l,
		RootHash: root,
		Data:     txs[i],
		Proof:    *proofs[i],
	}
}

type TxProof struct {
	Index, Total int
	RootHash     []byte
	Data         Tx
	Proof        merkle.SimpleProof
}

func (tp TxProof) LeafHash() []byte {
	return tp.Data.Hash()
}

func (tp TxProof) Validate(dataHash []byte) error {
	if !bytes.Equal(dataHash, tp.RootHash) {
		return errors.New("Proof matches different data hash")
	}

	valid := tp.Proof.Verify(tp.Index, tp.Total, tp.LeafHash(), tp.RootHash)
	if !valid {
		return errors.New("Proof is not internally consistent")
	}
	return nil
}
