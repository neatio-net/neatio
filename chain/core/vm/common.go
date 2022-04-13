package vm

import (
	"math/big"

	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/math"
)

func calcMemSize64(off, l *big.Int) (uint64, bool) {
	if !l.IsUint64() {
		return 0, true
	}
	return calcMemSize64WithUint(off, l.Uint64())
}

func calcMemSize64WithUint(off *big.Int, length64 uint64) (uint64, bool) {

	if length64 == 0 {
		return 0, false
	}

	if !off.IsUint64() {
		return 0, true
	}
	offset64 := off.Uint64()
	val := offset64 + length64

	return val, val < offset64
}

func getData(data []byte, start uint64, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return common.RightPadBytes(data[start:end], int(size))
}

func getDataBig(data []byte, start *big.Int, size *big.Int) []byte {
	dlen := big.NewInt(int64(len(data)))

	s := math.BigMin(start, dlen)
	e := math.BigMin(new(big.Int).Add(s, size), dlen)
	return common.RightPadBytes(data[s.Uint64():e.Uint64()], int(size.Uint64()))
}

func bigUint64(v *big.Int) (uint64, bool) {
	return v.Uint64(), !v.IsUint64()
}

func toWordSize(size uint64) uint64 {
	if size > math.MaxUint64-31 {
		return math.MaxUint64/32 + 1
	}

	return (size + 31) / 32
}

func allZero(b []byte) bool {
	for _, byte := range b {
		if byte != 0 {
			return false
		}
	}
	return true
}
