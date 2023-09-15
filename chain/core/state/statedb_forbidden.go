package state

import (
	"bytes"
	"fmt"
	"io"
	"sort"

	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/rlp"
)

// ----- banned Set

// MarkAddressBanned adds the specified object to the dirty map
func (self *StateDB) MarkAddressBanned(addr common.Address) {
	if _, exist := self.GetBannedSet()[addr]; !exist {
		self.bannedSet[addr] = struct{}{}
		self.bannedSetDirty = true
	}
}

func (self *StateDB) GetBannedSet() BannedSet {
	if len(self.bannedSet) != 0 {
		return self.bannedSet
	}
	// Try to get from Trie
	enc, err := self.trie.TryGet(bannedSetKey)
	if err != nil {
		self.setError(err)
		return nil
	}
	var value BannedSet
	if len(enc) > 0 {
		err := rlp.DecodeBytes(enc, &value)
		if err != nil {
			self.setError(err)
		}
		self.bannedSet = value
	}
	return value
}

func (self *StateDB) commitBannedSet() {
	data, err := rlp.EncodeToBytes(self.bannedSet)
	if err != nil {
		panic(fmt.Errorf("can't encode banned set : %v", err))
	}
	self.setError(self.trie.TryUpdate(bannedSetKey, data))
}

func (self *StateDB) ClearBannedSetByAddress(addr common.Address) {
	delete(self.bannedSet, addr)
	self.bannedSetDirty = true
}

// Store the Banned Address Set

var bannedSetKey = []byte("BannedSet")

type BannedSet map[common.Address]struct{}

func (set BannedSet) EncodeRLP(w io.Writer) error {
	var list []common.Address
	for addr := range set {
		list = append(list, addr)
	}
	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].Bytes(), list[j].Bytes()) == 1
	})
	return rlp.Encode(w, list)
}

func (set *BannedSet) DecodeRLP(s *rlp.Stream) error {
	var list []common.Address
	if err := s.Decode(&list); err != nil {
		return err
	}
	bannedSet := make(BannedSet, len(list))
	for _, addr := range list {
		bannedSet[addr] = struct{}{}
	}
	*set = bannedSet
	return nil
}
