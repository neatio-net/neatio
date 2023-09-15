package common

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"

	"github.com/nio-net/nio/utilities/common/hexutil"
)

const (
	HashLength = 32

	NEATAddressLength = 32
)

var (
	hashT    = reflect.TypeOf(Hash{})
	addressT = reflect.TypeOf(Address{})
)

type Hash [HashLength]byte

func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}
func StringToHash(s string) Hash { return BytesToHash([]byte(s)) }
func BigToHash(b *big.Int) Hash  { return BytesToHash(b.Bytes()) }
func HexToHash(s string) Hash    { return BytesToHash(FromHex(s)) }

func (h Hash) Str() string   { return string(h[:]) }
func (h Hash) Bytes() []byte { return h[:] }
func (h Hash) Big() *big.Int { return new(big.Int).SetBytes(h[:]) }
func (h Hash) Hex() string   { return hexutil.Encode(h[:]) }

func (h Hash) TerminalString() string {
	return fmt.Sprintf("%x…%x", h[:3], h[29:])
}

func (h Hash) String() string {
	return h.Hex()
}

func (h Hash) Format(s fmt.State, c rune) {
	fmt.Fprintf(s, "%"+string(c), h[:])
}

func (h *Hash) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("Hash", input, h[:])
}

func (h *Hash) UnmarshalJSON(input []byte) error {
	return hexutil.UnmarshalFixedJSON(hashT, input, h[:])
}

func (h Hash) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
}

func (h *Hash) SetString(s string) { h.SetBytes([]byte(s)) }

func (h *Hash) Set(other Hash) {
	for i, v := range other {
		h[i] = v
	}
}

func (h Hash) Generate(rand *rand.Rand, size int) reflect.Value {
	m := rand.Intn(len(h))
	for i := len(h) - 1; i > m; i-- {
		h[i] = byte(rand.Uint32())
	}
	return reflect.ValueOf(h)
}

func EmptyHash(h Hash) bool {
	return h == Hash{}
}

type UnprefixedHash Hash

func (h *UnprefixedHash) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedUnprefixedText("UnprefixedHash", input, h[:])
}

func (h UnprefixedHash) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(h[:])), nil
}

type Address [NEATAddressLength]byte

func BytesToAddress(b []byte) Address {
	var a Address
	a.SetBytes(b)
	return a
}
func StringToAddress(s string) Address { return BytesToAddress([]byte(s)) }
func BigToAddress(b *big.Int) Address  { return BytesToAddress(b.Bytes()) }
func HexToAddress(s string) Address    { return BytesToAddress(FromHex(s)) }

func IsHexAddress(s string) bool {
	if hasHexPrefix(s) {
		s = s[2:]
	}

	return len(s) == 2*NEATAddressLength && isHex(s)
}

func (a Address) Str() string   { return string(a[:]) }
func (a Address) Bytes() []byte { return a[:] }
func (a Address) Big() *big.Int { return new(big.Int).SetBytes(a[:]) }
func (a Address) Hash() Hash    { return BytesToHash(a[:]) }

func (a Address) Hex() string {
	unchecksummed := hex.EncodeToString(a[:])
	result := []byte(unchecksummed)
	return "0x" + string(result)
}

func (a Address) String() string {
	return string(a[:])
}

func (a Address) Format(s fmt.State, c rune) {
	fmt.Fprintf(s, "%"+string(c), a[:])
}

func (a *Address) SetBytes(b []byte) {
	if len(b) > NEATAddressLength {
		b = b[len(b)-NEATAddressLength:]
	}
	copy(a[NEATAddressLength-len(b):], b)
}

func (a *Address) SetString(s string) { a.SetBytes([]byte(s)) }

func (a *Address) Set(other Address) {
	for i, v := range other {
		a[i] = v
	}
}

func (a Address) MarshalText() ([]byte, error) {

	return hexutil.Bytes(a[:]).MarshalText()
}

func (a *Address) UnmarshalText(input []byte) error {
	fmt.Printf("types Address UnmarshalText address=%v, input=%v\n", a, input)
	return hexutil.UnmarshalFixedText("Address", input, a[:])
}

func (a *Address) UnmarshalJSON(input []byte) error {

	return hexutil.UnmarshalAddrFixedJSON(addressT, input, a[:])
}

type UnprefixedAddress Address

func (a *UnprefixedAddress) UnmarshalText(input []byte) error {

	return hexutil.UnmarshalFixedUnprefixedText("UnprefixedAddress", input, a[:])
}

func (a UnprefixedAddress) MarshalText() ([]byte, error) {

	return []byte(hex.EncodeToString(a[:])), nil
}
