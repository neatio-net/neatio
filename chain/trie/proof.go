package trie

import (
	"bytes"
	"fmt"

	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/rlp"
)

func (t *Trie) Prove(key []byte, fromLevel uint, proofDb neatdb.Writer) error {

	key = keybytesToHex(key)
	var nodes []node
	tn := t.root
	for len(key) > 0 && tn != nil {
		switch n := tn.(type) {
		case *shortNode:
			if len(key) < len(n.Key) || !bytes.Equal(n.Key, key[:len(n.Key)]) {

				tn = nil
			} else {
				tn = n.Val
				key = key[len(n.Key):]
			}
			nodes = append(nodes, n)
		case *fullNode:
			tn = n.Children[key[0]]
			key = key[1:]
			nodes = append(nodes, n)
		case hashNode:
			var err error
			tn, err = t.resolveHash(n, nil)
			if err != nil {
				log.Error(fmt.Sprintf("Unhandled trie error: %v", err))
				return err
			}
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", tn, tn))
		}
	}
	hasher := newHasher(nil)
	defer returnHasherToPool(hasher)

	for i, n := range nodes {

		n, _, _ = hasher.hashChildren(n, nil)
		hn, _ := hasher.store(n, nil, false)
		if hash, ok := hn.(hashNode); ok || i == 0 {

			if fromLevel > 0 {
				fromLevel--
			} else {
				enc, _ := rlp.EncodeToBytes(n)
				if !ok {
					hash = hasher.makeHashNode(enc)
				}
				proofDb.Put(hash, enc)
			}
		}
	}
	return nil
}

func (t *SecureTrie) Prove(key []byte, fromLevel uint, proofDb neatdb.Writer) error {
	return t.trie.Prove(key, fromLevel, proofDb)
}

func VerifyProof(rootHash common.Hash, key []byte, proofDb neatdb.Reader) (value []byte, nodes int, err error) {
	key = keybytesToHex(key)
	wantHash := rootHash
	for i := 0; ; i++ {
		buf, _ := proofDb.Get(wantHash[:])
		if buf == nil {
			return nil, i, fmt.Errorf("proof node %d (hash %064x) missing", i, wantHash)
		}
		n, err := decodeNode(wantHash[:], buf)
		if err != nil {
			return nil, i, fmt.Errorf("bad proof node %d: %v", i, err)
		}
		keyrest, cld := get(n, key)
		switch cld := cld.(type) {
		case nil:

			return nil, i, nil
		case hashNode:
			key = keyrest
			copy(wantHash[:], cld)
		case valueNode:
			return cld, i + 1, nil
		}
	}
}

func get(tn node, key []byte) ([]byte, node) {
	for {
		switch n := tn.(type) {
		case *shortNode:
			if len(key) < len(n.Key) || !bytes.Equal(n.Key, key[:len(n.Key)]) {
				return nil, nil
			}
			tn = n.Val
			key = key[len(n.Key):]
		case *fullNode:
			tn = n.Children[key[0]]
			key = key[1:]
		case hashNode:
			return key, n
		case nil:
			return key, nil
		case valueNode:
			return nil, n
		default:
			panic(fmt.Sprintf("%T: invalid node: %v", tn, tn))
		}
	}
}
