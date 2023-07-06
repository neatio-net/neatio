package neatcon

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
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
