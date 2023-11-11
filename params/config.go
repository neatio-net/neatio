package params

import (
	"fmt"
	"math/big"

	"github.com/neatio-net/neatio/chain/log"

	"github.com/neatio-net/neatio/utilities/common"
	"github.com/neatio-net/neatio/utilities/crypto"
)

var (
	MainnetGenesisHash = common.HexToHash("0x8f6eca8c3283bf4c65318888e0a9da2e27a30d6904a8011c85cdb26894d5f792")
	TestnetGenesisHash = common.HexToHash("0x473e0300dc12441d4d7450033a4ca4ba5c003547f6ffb7ef6cdeb2d02f475c39")
)

var (
	MainnetChainConfig = &ChainConfig{
		NeatChainId:         "neatio",
		ChainId:             big.NewInt(1001),
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		ConstantinopleBlock: nil,
		NeatCon: &NeatConConfig{
			Epoch:          30000,
			ProposerPolicy: 0,
		},
	}

	TestnetChainConfig = &ChainConfig{
		NeatChainId:         "testnet",
		ChainId:             big.NewInt(525),
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		ConstantinopleBlock: nil,
		NeatCon: &NeatConConfig{
			Epoch:          30000,
			ProposerPolicy: 0,
		},
	}

	TestChainConfig = &ChainConfig{"", big.NewInt(1), big.NewInt(0), big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, nil}
	TestRules       = TestChainConfig.Rules(new(big.Int))
)

func init() {

}

type ChainConfig struct {
	NeatChainId string   `json:"neatChainId"`
	ChainId     *big.Int `json:"chainId"`

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"`

	EIP150Block *big.Int    `json:"eip150Block,omitempty"`
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`

	EIP155Block *big.Int `json:"eip155Block,omitempty"`
	EIP158Block *big.Int `json:"eip158Block,omitempty"`

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"`
	PetersburgBlock     *big.Int `json:"petersburgBlock,omitempty"`
	IstanbulBlock       *big.Int `json:"istanbulBlock,omitempty"`

	NeatCon *NeatConConfig `json:"neatcon,omitempty"`

	ChainLogger log.Logger `json:"-"`
}

type NeatConConfig struct {
	Epoch          uint64 `json:"epoch"`
	ProposerPolicy uint64 `json:"policy"`
}

func (c *NeatConConfig) String() string {
	return "neatcon"
}

func NewSideChainConfig(sideChainID string) *ChainConfig {
	config := &ChainConfig{
		NeatChainId:         sideChainID,
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		NeatCon: &NeatConConfig{
			Epoch:          30000,
			ProposerPolicy: 0,
		},
	}

	digest := crypto.Keccak256([]byte(config.NeatChainId))
	config.ChainId = new(big.Int).SetBytes(digest[:])

	return config
}

func (c *ChainConfig) String() string {
	var engine interface{}
	switch {
	case c.NeatCon != nil:
		engine = c.NeatCon
	default:
		engine = "unknown"
	}
	return fmt.Sprintf("{NeatChainId: %s ChainID: %v Homestead: %v  EIP150: %v EIP155: %v EIP158: %v Byzantium: %v Constantinople: %v Petersburg: %v Istanbul: %v Engine: %v}",
		c.NeatChainId,
		c.ChainId,
		c.HomesteadBlock,
		c.EIP150Block,
		c.EIP155Block,
		c.EIP158Block,
		c.ByzantiumBlock,
		c.ConstantinopleBlock,
		c.PetersburgBlock,
		c.IstanbulBlock,
		engine,
	)
}

func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(c.HomesteadBlock, num)
}

func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(c.EIP150Block, num)
}

func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return isForked(c.EIP155Block, num)
}

func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return isForked(c.EIP158Block, num)
}

func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return isForked(c.ByzantiumBlock, num)
}

func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isForked(c.ConstantinopleBlock, num)
}

func (c *ChainConfig) IsPetersburg(num *big.Int) bool {
	return isForked(c.PetersburgBlock, num) || c.PetersburgBlock == nil && isForked(c.ConstantinopleBlock, num)
}

func (c *ChainConfig) IsIstanbul(num *big.Int) bool {
	return isForked(c.IstanbulBlock, num)
}

func (c *ChainConfig) IsEWASM(num *big.Int) bool {
	return false
}

func (c *ChainConfig) IsMainChain() bool {
	return c.NeatChainId == MainnetChainConfig.NeatChainId || c.NeatChainId == TestnetChainConfig.NeatChainId
}

func IsMainChain(chainId string) bool {
	return chainId == MainnetChainConfig.NeatChainId || chainId == TestnetChainConfig.NeatChainId
}

func (c *ChainConfig) GasTable(num *big.Int) GasTable {
	if num == nil {
		return GasTableHomestead
	}
	switch {
	case c.IsEIP158(num):
		return GasTableEIP158
	case c.IsEIP150(num):
		return GasTableEIP150
	default:
		return GasTableHomestead
	}
}

func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

	var lasterr *ConfigCompatError
	for {
		err := c.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	if isForkIncompatible(c.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", c.HomesteadBlock, newcfg.HomesteadBlock)
	}
	if isForkIncompatible(c.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", c.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(c.EIP155Block, newcfg.EIP155Block, head) {
		return newCompatError("EIP155 fork block", c.EIP155Block, newcfg.EIP155Block)
	}
	if isForkIncompatible(c.EIP158Block, newcfg.EIP158Block, head) {
		return newCompatError("EIP158 fork block", c.EIP158Block, newcfg.EIP158Block)
	}
	if c.IsEIP158(head) && !configNumEqual(c.ChainId, newcfg.ChainId) {
		return newCompatError("EIP158 chain ID", c.EIP158Block, newcfg.EIP158Block)
	}
	if isForkIncompatible(c.ByzantiumBlock, newcfg.ByzantiumBlock, head) {
		return newCompatError("Byzantium fork block", c.ByzantiumBlock, newcfg.ByzantiumBlock)
	}
	if isForkIncompatible(c.ConstantinopleBlock, newcfg.ConstantinopleBlock, head) {
		return newCompatError("Constantinople fork block", c.ConstantinopleBlock, newcfg.ConstantinopleBlock)
	}
	if isForkIncompatible(c.PetersburgBlock, newcfg.PetersburgBlock, head) {
		return newCompatError("Petersburg fork block", c.PetersburgBlock, newcfg.PetersburgBlock)
	}
	if isForkIncompatible(c.IstanbulBlock, newcfg.IstanbulBlock, head) {
		return newCompatError("Istanbul fork block", c.IstanbulBlock, newcfg.IstanbulBlock)
	}
	return nil
}

func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}

func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}

type ConfigCompatError struct {
	What string

	StoredConfig, NewConfig *big.Int

	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

type Rules struct {
	ChainId                                                 *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158               bool
	IsByzantium, IsConstantinople, IsPetersburg, IsIstanbul bool
}

func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainId := c.ChainId
	if chainId == nil {
		chainId = new(big.Int)
	}
	return Rules{
		ChainId:          new(big.Int).Set(chainId),
		IsHomestead:      c.IsHomestead(num),
		IsEIP150:         c.IsEIP150(num),
		IsEIP155:         c.IsEIP155(num),
		IsEIP158:         c.IsEIP158(num),
		IsByzantium:      c.IsByzantium(num),
		IsConstantinople: c.IsConstantinople(num),
		IsPetersburg:     c.IsPetersburg(num),
		IsIstanbul:       c.IsIstanbul(num),
	}
}
