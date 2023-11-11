package vm

import (
	"math"
	"testing"
)

func TestMemoryGasCost(t *testing.T) {
	tests := []struct {
		size     uint64
		cost     uint64
		overflow bool
	}{
		{0x1fffffffe0, 36028809887088637, false},
		{0x1fffffffe1, 0, true},
	}
	for i, tt := range tests {
		v, err := memoryGasCost(&Memory{}, tt.size)
		if (err == errGasUintOverflow) != tt.overflow {
			t.Errorf("test %d: overflow mismatch: have %v, want %v", i, err == errGasUintOverflow, tt.overflow)
		}
		if v != tt.cost {
			t.Errorf("test %d: gas cost mismatch: have %v, want %v", i, v, tt.cost)
		}
	}
}

var eip2200Tests = []struct {
	original byte
	gaspool  uint64
	input    string
	used     uint64
	refund   uint64
	failure  error
}{
	{0, math.MaxUint64, "0x60006000556000600055", 1612, 0, nil},
	{0, math.MaxUint64, "0x60006000556001600055", 20812, 0, nil},
	{0, math.MaxUint64, "0x60016000556000600055", 20812, 19200, nil},
	{0, math.MaxUint64, "0x60016000556002600055", 20812, 0, nil},
	{0, math.MaxUint64, "0x60016000556001600055", 20812, 0, nil},
	{1, math.MaxUint64, "0x60006000556000600055", 5812, 15000, nil},
	{1, math.MaxUint64, "0x60006000556001600055", 5812, 4200, nil},
	{1, math.MaxUint64, "0x60006000556002600055", 5812, 0, nil},
	{1, math.MaxUint64, "0x60026000556000600055", 5812, 15000, nil},
	{1, math.MaxUint64, "0x60026000556003600055", 5812, 0, nil},
	{1, math.MaxUint64, "0x60026000556001600055", 5812, 4200, nil},
	{1, math.MaxUint64, "0x60026000556002600055", 5812, 0, nil},
	{1, math.MaxUint64, "0x60016000556000600055", 5812, 15000, nil},
	{1, math.MaxUint64, "0x60016000556002600055", 5812, 0, nil},
	{1, math.MaxUint64, "0x60016000556001600055", 1612, 0, nil},
	{0, math.MaxUint64, "0x600160005560006000556001600055", 40818, 19200, nil},
	{1, math.MaxUint64, "0x600060005560016000556000600055", 10818, 19200, nil},
	{1, 2306, "0x6001600055", 2306, 0, ErrOutOfGas},
	{1, 2307, "0x6001600055", 806, 0, nil},
}
