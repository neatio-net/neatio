package params

type GasTable struct {
	ExtcodeSize uint64
	ExtcodeCopy uint64
	ExtcodeHash uint64
	Balance     uint64
	SLoad       uint64
	Calls       uint64
	Suicide     uint64

	ExpByte uint64

	CreateBySuicide uint64
}

var (
	GasTableHomestead = GasTable{
		ExtcodeSize: 1,
		ExtcodeCopy: 1,
		Balance:     1,
		SLoad:       1,
		Calls:       1,
		Suicide:     0,
		ExpByte:     1,
	}

	GasTableEIP150 = GasTable{
		ExtcodeSize: 1,
		ExtcodeCopy: 1,
		Balance:     1,
		SLoad:       1,
		Calls:       1,
		Suicide:     1,
		ExpByte:     110,

		CreateBySuicide: 1,
	}

	GasTableEIP158 = GasTable{
		ExtcodeSize: 1,
		ExtcodeCopy: 1,
		Balance:     1,
		SLoad:       1,
		Calls:       1,
		Suicide:     1,
		ExpByte:     1,

		CreateBySuicide: 1,
	}

	GasTableConstantinople = GasTable{
		ExtcodeSize: 1,
		ExtcodeCopy: 1,
		ExtcodeHash: 1,
		Balance:     1,
		SLoad:       1,
		Calls:       1,
		Suicide:     1,
		ExpByte:     1,

		CreateBySuicide: 1,
	}
)
