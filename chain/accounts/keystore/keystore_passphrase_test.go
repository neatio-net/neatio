package keystore

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/nio-net/nio/utilities/crypto"

	"github.com/nio-net/nio/utilities/common"
)

const (
	veryLightScryptN = 2
	veryLightScryptP = 1
)

func TestKeyEncryptDecrypt(t *testing.T) {
	keyjson, err := ioutil.ReadFile("testdata/very-light-scrypt.json")
	if err != nil {
		t.Fatal(err)
	}
	password := ""
	address := common.HexToAddress("45dea0fb0bba44f4fcf290bba71fd57d7117cbb8")

	for i := 0; i < 3; i++ {

		if _, err := DecryptKey(keyjson, password+"bad"); err == nil {
			t.Errorf("test %d: json key decrypted with bad password", i)
		}

		key, err := DecryptKey(keyjson, password)
		if err != nil {
			t.Fatalf("test %d: json key failed to decrypt: %v", i, err)
		}
		if key.Address != address {
			t.Errorf("test %d: key address mismatch: have %x, want %x", i, key.Address, address)
		}

		password += "new data appended"
		if keyjson, err = EncryptKey(key, password, veryLightScryptN, veryLightScryptP); err != nil {
			t.Errorf("test %d: failed to recrypt key %v", i, err)
		}
	}
}

func TestEncryptKey(t *testing.T) {

}

func TestDecryptKey(t *testing.T) {
	keyjson, err := ioutil.ReadFile("testdata/keystore/UTC--2019-10-17T06-41-37.816846000Z--3K7YBykphE6N8jFGVbNAWfvor94i9nigU8")
	if err != nil {
		t.Fatal(err)
	}

	password := "neatio"
	address := common.StringToAddress("3K7YBykphE6N8jFGVbNAWfvor94i9nigU8")
	key, err := DecryptKey(keyjson, password)
	fmt.Printf("private key %x\n", crypto.FromECDSA(key.PrivateKey))
	if err != nil {
		t.Fatalf("json key failed to decrypt err=%v\n", err)
	}

	fmt.Printf("address are key.Address %x, address %x\n", key.Address, address)
	if key.Address != address {
		t.Errorf("address mismatch: have %v, want %v\n", key.Address.String(), address.String())
	}
}
