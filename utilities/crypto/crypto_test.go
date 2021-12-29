package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/btcsuite/btcutil/base58"
	"github.com/neatlab/neatio/utilities/common/hexutil"

	"github.com/neatlab/neatio/utilities/common"
)

var testAddrHex = "970e8128ab834e8eac17ab8e3812f010678cf791"
var testNEATAddrHex = "3334536a7873394177526a356d437978374166784a755a783566393142714a353333"

var testNEATAddr = "NEATb437dSzaqRGxhTgW4qCq877ytYxb"
var testPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

func TestKeccak256Hash(t *testing.T) {
	msg := []byte("abc")
	exp, _ := hex.DecodeString("4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45")
	checkhash(t, "Sha3-256-array", func(in []byte) []byte { h := Keccak256Hash(in); return h[:] }, msg, exp)
}

func TestToECDSAErrors(t *testing.T) {
	if _, err := HexToECDSA("0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("HexToECDSA should've returned error")
	}
	if _, err := HexToECDSA("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"); err == nil {
		t.Fatal("HexToECDSA should've returned error")
	}
}

func BenchmarkSha3(b *testing.B) {
	a := []byte("hello world")
	for i := 0; i < b.N; i++ {
		Keccak256(a)
	}
}

func TestSign(t *testing.T) {
	key, _ := HexToECDSA(testPrivHex)
	addr := common.HexToAddress(testNEATAddrHex)

	msg := Keccak256([]byte("foo"))
	sig, err := Sign(msg, key)
	if err != nil {
		t.Errorf("Sign error: %s", err)
	}
	recoveredPub, err := Ecrecover(msg, sig)
	if err != nil {
		t.Errorf("ECRecover error: %s", err)
	}
	pubKey, _ := UnmarshalPubkey(recoveredPub)
	recoveredAddr := PubkeyToAddress(*pubKey)
	if addr != recoveredAddr {
		t.Errorf("Address mismatch: want: %x have: %x", addr, recoveredAddr)
	}

	recoveredPub2, err := SigToPub(msg, sig)
	if err != nil {
		t.Errorf("ECRecover error: %s", err)
	}
	recoveredAddr2 := PubkeyToAddress(*recoveredPub2)
	if addr != recoveredAddr2 {
		t.Errorf("Address mismatch: want: %x have: %x", addr, recoveredAddr2)
	}
}

func TestUnmarshalPubkey(t *testing.T) {
	key, err := UnmarshalPubkey(nil)
	if err != errInvalidPubkey || key != nil {
		t.Fatalf("expected error, got %v, %v", err, key)
	}
	key, err = UnmarshalPubkey([]byte{1, 2, 3})
	if err != errInvalidPubkey || key != nil {
		t.Fatalf("expected error, got %v, %v", err, key)
	}

	var (
		enc, _ = hex.DecodeString("04760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1b01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d")
		dec    = &ecdsa.PublicKey{
			Curve: S256(),
			X:     hexutil.MustDecodeBig("0x760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1"),
			Y:     hexutil.MustDecodeBig("0xb01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d"),
		}
	)
	key, err = UnmarshalPubkey(enc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(key, dec) {
		t.Fatal("wrong result")
	}
}

func TestInvalidSign(t *testing.T) {
	if _, err := Sign(make([]byte, 1), nil); err == nil {
		t.Errorf("expected sign with hash 1 byte to error")
	}
	if _, err := Sign(make([]byte, 33), nil); err == nil {
		t.Errorf("expected sign with hash 33 byte to error")
	}
}

func TestNewContractAddress(t *testing.T) {
	key, _ := HexToECDSA(testPrivHex)
	addr := common.HexToAddress(testNEATAddrHex)
	fmt.Printf("byte addr=%v\n", addr)
	genAddr := PubkeyToAddress(key.PublicKey)
	fmt.Printf("gen addr=%v\n", addr)
	checkAddr(t, genAddr, addr)

	caddr0 := CreateAddress(addr, 0)
	caddr1 := CreateAddress(addr, 1)
	caddr2 := CreateAddress(addr, 2)
	checkAddr(t, common.HexToAddress("3343384b35786b757666344431674c5376684346413467674b73506b7268316f4c31"), caddr0)
	checkAddr(t, common.HexToAddress("334b713575554c4c594e6e65546831544e7a4352767377626e554a55334a6d6f3773"), caddr1)
	checkAddr(t, common.HexToAddress("3339416a5166364c48454346596a316e45566a67414d675250723578414d69334b67"), caddr2)
}

func TestLoadECDSAFile(t *testing.T) {
	keyBytes := common.FromHex(testPrivHex)
	fileName0 := "test_key0"
	fileName1 := "test_key1"
	checkKey := func(k *ecdsa.PrivateKey) {
		checkAddr(t, PubkeyToAddress(k.PublicKey), common.HexToAddress(testAddrHex))
		loadedKeyBytes := FromECDSA(k)
		if !bytes.Equal(loadedKeyBytes, keyBytes) {
			t.Fatalf("private key mismatch: want: %x have: %x", keyBytes, loadedKeyBytes)
		}
	}

	ioutil.WriteFile(fileName0, []byte(testPrivHex), 0600)
	defer os.Remove(fileName0)

	key0, err := LoadECDSA(fileName0)
	if err != nil {
		t.Fatal(err)
	}
	checkKey(key0)

	err = SaveECDSA(fileName1, key0)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fileName1)

	key1, err := LoadECDSA(fileName1)
	if err != nil {
		t.Fatal(err)
	}
	checkKey(key1)
}

func TestValidateSignatureValues(t *testing.T) {
	check := func(expected bool, v byte, r, s *big.Int) {
		if ValidateSignatureValues(v, r, s, false) != expected {
			t.Errorf("mismatch for v: %d r: %d s: %d want: %v", v, r, s, expected)
		}
	}
	minusOne := big.NewInt(-1)
	one := common.Big1
	zero := common.Big0
	secp256k1nMinus1 := new(big.Int).Sub(secp256k1N, common.Big1)

	check(true, 0, one, one)
	check(true, 1, one, one)

	check(false, 2, one, one)
	check(false, 3, one, one)

	check(false, 2, zero, zero)
	check(false, 2, zero, one)
	check(false, 2, one, zero)
	check(false, 2, one, one)

	check(false, 0, zero, zero)
	check(false, 0, zero, one)
	check(false, 0, one, zero)

	check(false, 1, zero, zero)
	check(false, 1, zero, one)
	check(false, 1, one, zero)

	check(true, 0, secp256k1nMinus1, secp256k1nMinus1)

	check(false, 0, secp256k1N, secp256k1nMinus1)
	check(false, 0, secp256k1nMinus1, secp256k1N)
	check(false, 0, secp256k1N, secp256k1N)

	check(false, 0, minusOne, one)
	check(false, 0, one, minusOne)
}

func checkhash(t *testing.T, name string, f func([]byte) []byte, msg, exp []byte) {
	sum := f(msg)
	if !bytes.Equal(exp, sum) {
		t.Fatalf("hash %s mismatch: want: %x have: %x", name, exp, sum)
	}
}

func checkAddr(t *testing.T, addr0, addr1 common.Address) {
	if addr0 != addr1 {
		t.Fatalf("address mismatch: want: %x have: %x", addr0, addr1)
	}
}

func TestPythonIntegration(t *testing.T) {
	kh := "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"
	k0, _ := HexToECDSA(kh)

	msg0 := Keccak256([]byte("foo"))
	sig0, _ := Sign(msg0, k0)

	msg1 := common.FromHex("00000000000000000000000000000000")
	sig1, _ := Sign(msg0, k0)

	t.Logf("msg: %x, privkey: %s sig: %x\n", msg0, kh, sig0)
	t.Logf("msg: %x, privkey: %s sig: %x\n", msg1, kh, sig1)
}

func TestNewNEATAddr(t *testing.T) {
	key, _ := HexToECDSA(testPrivHex)
	pubKeyBytes := FromECDSAPub(&key.PublicKey)
	fmt.Printf("pubkeybytes=%v\n\n", pubKeyBytes)
	fmt.Printf("pubkeyHex=%v\n\n", hexutil.Encode(pubKeyBytes))

	pubkey := ToECDSAPub(pubKeyBytes)
	fmt.Printf("toecdsapub pubkey=%v\n\n", pubkey)

	fmt.Printf("pubKey=%v\n", key.PublicKey)
	fmt.Printf("x=%v\n", key.PublicKey.X.Bytes())
	fmt.Printf("y=%v\n\n", key.PublicKey.Y.Bytes())

	addr := NewNEATScriptAddr(pubKeyBytes)
	fmt.Printf("address=%v\n", addr)

	pubByte, _ := hexutil.Decode("0x04ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	pubAddr := NewNEATPubkeyAddr(pubByte)
	fmt.Printf("0x040000 address %v\n", pubAddr)

	data, _ := hexutil.Decode("0x0095027ab391b1a5327c6e64548e9340f1212b0b5b")
	fmt.Printf("data %v\n", data)
	checkByte := calcHash(calcHash(data, sha256.New()), sha256.New())
	fmt.Printf("checkByte %v\n", checkByte[:4])
	preDataCheck := append(data[:], checkByte[:4]...)
	fmt.Printf("preDataCheck %v\n", preDataCheck)
	bs58checkAddress := base58.Encode(preDataCheck)
	fmt.Printf("bs58checkAddress %v\n", bs58checkAddress)

	inputStr := "1FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	result, version, _ := CheckDecode(inputStr)
	fmt.Printf("result %v, version %v\n", hexutil.Encode(result), version)

	inputStr2 := "1EaterAddressDontSendAssetToFFFFFF"
	result2, version2, _ := CheckDecode(inputStr2)
	fmt.Printf("result %v, version %v\n", hexutil.Encode(result2), version2)

	fffByte, _ := hexutil.Decode("0x00ffffffffffffffffffffffffffffffffffffffffffffffff")
	fffStr := base58.Encode(fffByte)
	fmt.Printf("fffStr %v\n", fffStr)

	binAddr := common.StringToAddress(addr)
	fmt.Printf("binary address=%v\n", binAddr)

	hexAddr := common.BytesToAddress(binAddr[:]).Hex()
	fmt.Printf("hex address=%v\n", hexAddr)

	strAddr := binAddr.String()
	fmt.Printf("string address=%v\n\n", strAddr)

	checkNEATAddr(t, addr, testNEATAddr)
}

func BenchmarkCreateNEATAddress(b *testing.B) {
	for i := 0; i < 100; i++ {
		key, _ := GenerateKey()
		addr := NewNEATScriptAddr(FromECDSAPub(&key.PublicKey))
		fmt.Printf("NEAT address %v\n", addr)
		addrLen := len([]byte(addr))
		if addrLen != common.NEATAddressLength {
			b.Errorf("NEAT address %v lenght mismatch want %v, but %v\n", addr, common.NEATAddressLength, addrLen)
		}
	}
}

type addressTest struct {
	Address string
	Valid   bool
}

var addressList = []*addressTest{
	{Address: "NEATb437dSzaqRGxhTgW4qCq877ytYxb", Valid: true},
	{Address: "NEATb437dSzaqRGxhTgW4qCq877ytYx", Valid: false},
	{Address: "NEAHb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "nEATb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NeATb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEaTb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEAtb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "neaTb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "neATb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NeaTb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEatb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "nEAtb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NeAtb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "nEatb437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "b437dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEAT", Valid: false},
	{Address: "NEATb43ldSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEATb43IdSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEATb430dSzaqRGxhTgW4qCq877ytYxb", Valid: false},
	{Address: "NEATb43OdSzaqRGxhTgW4qCq877ytYxb", Valid: false},
}

func TestValidateNEATAddress(t *testing.T) {
	for _, v := range addressList {
		b := ValidateNEATAddr(v.Address)
		if b == v.Valid {
			t.Log("pass")
		} else {
			t.Errorf("address %v invalid, want %v but %v", v.Address, v.Valid, b)
		}
	}

}

func TestHexToAddress(t *testing.T) {
	addr := common.HexToAddress("0x494e5433437046756b32634a31746539575a563177385933776b51436341355a")
	fmt.Printf("addr=%v\n", addr.String())
	fmt.Printf("addr=%v\n", len(addr))
	fmt.Printf("testNEATAddrHex=%v\n", []byte(testNEATAddrHex))
}

func checkNEATAddr(t *testing.T, addr0, addr1 string) {
	if addr0 != addr1 {
		t.Fatalf("address mismatch want: %s have: %s", addr0, addr1)
	}
}

func TestEthAddress(t *testing.T) {
	privateKeyHex := "c15c038a5a9f8f948a2ac0eb102c249e4ae1c4fa1e0971b50c63db46dc5fcf8b"
	privateKey, err := HexToECDSA(privateKeyHex)
	if err != nil {
		t.Fatalf("failed to decode private key %v\n", err)
	}

	publicKey := FromECDSAPub(&privateKey.PublicKey)

	ethAddress := hexutil.Encode(Keccak256(publicKey[1:])[12:])

	fmt.Printf("ethereum address %v\n", ethAddress)
}

func TestByte(t *testing.T) {
	name := "LikeToken"
	symbol := "LC"

	byte1 := hexutil.Encode([]byte(name))
	byte2 := hexutil.Encode([]byte(symbol))
	fmt.Printf("name %v, symbol %v\n", byte1, byte2)
}

var messageByte = []byte("")

func CheckDecode(input string) (result []byte, version byte, err error) {
	decoded := base58.Decode(input)
	if len(decoded) < 5 {
		return nil, 0, nil
	}
	version = decoded[0]
	var cksum [4]byte
	copy(cksum[:], decoded[len(decoded)-4:])

	payload := decoded[1 : len(decoded)-4]
	result = append(result, payload...)
	return
}
