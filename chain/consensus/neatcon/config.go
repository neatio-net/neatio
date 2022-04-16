package neatcon

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
)

var (
	BigStr1 = "362209239056230863865135"
	BigStr2 = "816822002239807650213698"
)

type Config struct {
	RequestTimeout uint64         `toml:",omitempty"`
	BlockPeriod    uint64         `toml:",omitempty"`
	ProposerPolicy ProposerPolicy `toml:",omitempty"`
	Epoch          uint64         `toml:",omitempty"`
}

var DefaultConfig = &Config{
	RequestTimeout: 10000,
	BlockPeriod:    1,
	ProposerPolicy: RoundRobin,
	Epoch:          30000,
}
