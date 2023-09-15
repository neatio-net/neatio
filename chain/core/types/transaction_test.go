// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/nio-net/nio/utilities/common/hexutil"

	neatAbi "github.com/nio-net/nio/neatabi/abi"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/crypto"
	"github.com/nio-net/nio/utilities/rlp"
)

// The values in those tests are from the Transaction Tests
// at github.com/ethereum/tests.
var (
	emptyTx = NewTransaction(
		0,
		common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
		big.NewInt(0), 0, big.NewInt(0),
		nil,
	)

	rightvrsTx, _ = NewTransaction(
		3,
		common.HexToAddress("b94f5374fce5edbc8e2a8697c15331677e6ebf0b"),
		big.NewInt(10),
		2000,
		big.NewInt(1),
		common.FromHex("5544"),
	).WithSignature(
		HomesteadSigner{},
		common.Hex2Bytes("98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a8887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a301"),
	)
)

func TestTransactionSigHash(t *testing.T) {
	var homestead HomesteadSigner
	if homestead.Hash(emptyTx) != common.HexToHash("c775b99e7ad12f50d819fcd602390467e28141316969f4b57f0626f74fe3b386") {
		t.Errorf("empty transaction hash mismatch, got %x", emptyTx.Hash())
	}
	if homestead.Hash(rightvrsTx) != common.HexToHash("fe7a79529ed5f7c3375d06b26b186a8644e0e16c373d7a12be41c62d6042b77a") {
		t.Errorf("RightVRS transaction hash mismatch, got %x", rightvrsTx.Hash())
	}
}

func TestTransactionEncode(t *testing.T) {
	txb, err := rlp.EncodeToBytes(rightvrsTx)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	should := common.FromHex("f86103018207d094b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a8255441ca098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa08887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a3")
	if !bytes.Equal(txb, should) {
		t.Errorf("encoded RLP mismatch, got %x", txb)
	}
}

func decodeTx(data []byte) (*Transaction, error) {
	var tx Transaction
	t, err := &tx, rlp.Decode(bytes.NewReader(data), &tx)

	return t, err
}

func defaultTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return key, addr
}

func TestRecipientEmpty(t *testing.T) {
	_, addr := defaultTestKey()
	tx, err := decodeTx(common.Hex2Bytes("f8498080808080011ca09b16de9d5bdee2cf56c28d16275a4da68cd30273e2525f3959f5d62557489921a0372ebd8fb3345f7db7b5a86d42e24d36e983e259b0664ceb8c227ec9af572f3d"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if addr != from {
		t.Error("derived address doesn't match")
	}
}

func TestRecipientNormal(t *testing.T) {
	_, addr := defaultTestKey()

	tx, err := decodeTx(common.Hex2Bytes("f85d80808094000000000000000000000000000000000000000080011ca0527c0d8f5c63f7b9f41324a7c8a563ee1190bcbf0dac8ab446291bdbf32f5c79a0552c4ef0a09a04395074dab9ed34d3fbfb843c2f2546cc30fe89ec143ca94ca6"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if addr != from {
		t.Error("derived address doesn't match")
	}
}

// Tests that transactions can be correctly sorted according to their price in
// decreasing order, but at the same time with increasing nonces when issued by
// the same account.
func TestTransactionPriceNonceSort(t *testing.T) {
	// Generate a batch of accounts to start with
	keys := make([]*ecdsa.PrivateKey, 25)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
	}

	signer := HomesteadSigner{}
	// Generate a batch of transactions with overlapping values, but shifted nonces
	groups := map[common.Address]Transactions{}
	for start, key := range keys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		for i := 0; i < 25; i++ {
			tx, _ := SignTx(NewTransaction(uint64(start+i), common.Address{}, big.NewInt(100), 100, big.NewInt(int64(start+i)), nil), signer, key)
			groups[addr] = append(groups[addr], tx)
		}
	}
	// Sort the transactions and cross check the nonce ordering
	txset := NewTransactionsByPriceAndNonce(signer, groups)

	txs := Transactions{}
	for tx := txset.Peek(); tx != nil; tx = txset.Peek() {
		txs = append(txs, tx)
		txset.Shift()
	}
	if len(txs) != 25*25 {
		t.Errorf("expected %d transactions, found %d", 25*25, len(txs))
	}
	for i, txi := range txs {
		fromi, _ := Sender(signer, txi)

		// Make sure the nonce order is valid
		for j, txj := range txs[i+1:] {
			fromj, _ := Sender(signer, txj)

			if fromi == fromj && txi.Nonce() > txj.Nonce() {
				t.Errorf("invalid nonce ordering: tx #%d (A=%x N=%v) < tx #%d (A=%x N=%v)", i, fromi[:4], txi.Nonce(), i+j, fromj[:4], txj.Nonce())
			}
		}
		// Find the previous and next nonce of this account
		prev, next := i-1, i+1
		for j := i - 1; j >= 0; j-- {
			if fromj, _ := Sender(signer, txs[j]); fromi == fromj {
				prev = j
				break
			}
		}
		for j := i + 1; j < len(txs); j++ {
			if fromj, _ := Sender(signer, txs[j]); fromi == fromj {
				next = j
				break
			}
		}
		// Make sure that in between the neighbor nonces, the transaction is correctly positioned price wise
		for j := prev + 1; j < next; j++ {
			fromj, _ := Sender(signer, txs[j])
			if j < i && txs[j].GasPrice().Cmp(txi.GasPrice()) < 0 {
				t.Errorf("invalid gasprice ordering: tx #%d (A=%x P=%v) < tx #%d (A=%x P=%v)", j, fromj[:4], txs[j].GasPrice(), i, fromi[:4], txi.GasPrice())
			}
			if j > i && txs[j].GasPrice().Cmp(txi.GasPrice()) > 0 {
				t.Errorf("invalid gasprice ordering: tx #%d (A=%x P=%v) > tx #%d (A=%x P=%v)", j, fromj[:4], txs[j].GasPrice(), i, fromi[:4], txi.GasPrice())
			}
		}
	}
}

// TestTransactionJSON tests serializing/de-serializing to/from JSON.
func TestTransactionJSON(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("could not generate key: %v", err)
	}
	signer := NewEIP155Signer(common.Big1)

	for i := uint64(0); i < 25; i++ {
		var tx *Transaction
		switch i % 2 {
		case 0:
			tx = NewTransaction(i, common.Address{1}, common.Big0, 1, common.Big2, []byte("abcdef"))
		case 1:
			tx = NewContractCreation(i, common.Big0, 1, common.Big2, []byte("abcdef"))
		}

		tx, err := SignTx(tx, signer, key)
		if err != nil {
			t.Fatalf("could not sign transaction: %v", err)
		}

		data, err := json.Marshal(tx)
		if err != nil {
			t.Errorf("json.Marshal failed: %v", err)
		}

		var parsedTx *Transaction
		if err := json.Unmarshal(data, &parsedTx); err != nil {
			t.Errorf("json.Unmarshal failed: %v", err)
		}

		// compare nonce, price, gaslimit, recipient, amount, payload, V, R, S
		if tx.Hash() != parsedTx.Hash() {
			t.Errorf("parsed tx differs from original tx, want %v, got %v", tx, parsedTx)
		}
		if tx.ChainId().Cmp(parsedTx.ChainId()) != 0 {
			t.Errorf("invalid chain id, want %d, got %d", tx.ChainId(), parsedTx.ChainId())
		}
	}
}

// TestTransactionJSON tests serializing/de-serializing to/from JSON.
func TestNewTransactionJSON(t *testing.T) {
	key, err := crypto.HexToECDSA("c15c038a5a9f8f948a2ac0eb102c249e4ae1c4fa1f0971b50c63db46dc5fcf8b")
	if err != nil {
		t.Fatalf("could not decode key: %v", err)
	}
	signer := NewEIP155Signer(common.Big1)

	//for i := uint64(0); i < 25; i++ {
	var tx *Transaction
	//switch i % 2 {
	//case 0:
	tx = NewTransaction(0, common.StringToAddress("NEATtWX5FxpBddhwxzde47H7dWorB6AA"), big.NewInt(1000000000000000000), 30000, big.NewInt(20000000000), []byte("0x"))
	//case 1:
	//	tx = NewContractCreation(i, common.Big0, 1, common.Big2, []byte("abcdef"))
	//}
	fmt.Printf("before sign tx %v\n", tx)
	tx, err = SignTx(tx, signer, key)
	if err != nil {
		t.Fatalf("could not sign transaction: %v", err)
	}
	fmt.Printf("after sign tx %v\n", tx)

	data, err := json.Marshal(tx)
	if err != nil {
		t.Errorf("json.Marshal failed: %v", err)
	}

	var parsedTx *Transaction
	if err := json.Unmarshal(data, &parsedTx); err != nil {
		t.Errorf("json.Unmarshal failed: %v", err)
	}

	// compare nonce, price, gaslimit, recipient, amount, payload, V, R, S
	if tx.Hash() != parsedTx.Hash() {
		t.Errorf("parsed tx differs from original tx, want %v, got %v", tx, parsedTx)
	}
	if tx.ChainId().Cmp(parsedTx.ChainId()) != 0 {
		t.Errorf("invalid chain id, want %d, got %d", tx.ChainId(), parsedTx.ChainId())
	}
	//}
}

func TestSignTx(t *testing.T) {
	key, err := crypto.HexToECDSA("c15c038a5a9f8f948a2ac0eb102c249e4ae1c4fa1e0971b50c63db46dc5fcf8b")
	if err != nil {
		t.Fatalf("could not decode key: %v", err)
	}
	signer := NewEIP155Signer(common.Big2) // mainnet 1，testnet 2
	address := common.StringToAddress("NEATtWX5FxpBddhwxzde47H7dWorB6AA")

	d := txdata{
		AccountNonce: 0,
		Recipient:    &address,
		Payload:      []byte(""),
		Amount:       big.NewInt(1),
		GasLimit:     100000,
		Price:        big.NewInt(10000000000),
		V:            new(big.Int),
		R:            new(big.Int),
		S:            new(big.Int),
	}

	tx := &Transaction{data: d}
	t.Logf("tx before sign  %v\n", tx)
	signTx, err := SignTx(tx, signer, key)
	if err != nil {
		t.Error(err)
	}
	t.Logf("tx after sign  %v\n", signTx)

	enc, err := rlp.EncodeToBytes(&signTx.data)
	if err != nil {
		t.Logf("rlp encode to byte err %v\n", err)
	}

	fmt.Printf("ecn %x\n", enc)
}

func TestDelegateTx(t *testing.T) {
	key, err := crypto.HexToECDSA("c15c038a5a9f8f948a2ac0eb102c249e4ae1c4fa1e0971b50c63db46dc5fcf8b")
	if err != nil {
		t.Fatalf("could not decode key: %v", err)
	}
	signer := NewEIP155Signer(common.Big2) // mainnet 1，testnet 2
	//address := common.StringToAddress("NEATioMiningSmartContractAddress")
	address := common.StringToAddress("NEATioMiningSmartContractAddress")

	input, err := neatAbi.ChainABI.Pack(neatAbi.Delegate.String(), common.StringToAddress("NEATioMiningSmartContractAddress"))
	fmt.Printf("delegate input %v\n", hexutil.Encode(input))
	if err != nil {
		t.Errorf("could not pack data, err %v\n", err)
	}

	d := txdata{
		AccountNonce: 1,
		Recipient:    &address,
		//Payload:      []byte(""),
		Payload:  input,
		Amount:   big.NewInt(1000000000000000000),
		GasLimit: 50000,
		Price:    big.NewInt(10000000000),
		V:        new(big.Int),
		R:        new(big.Int),
		S:        new(big.Int),
	}

	tx := &Transaction{data: d}
	t.Logf("tx before sign  %v\n", tx)
	signTx, err := SignTx(tx, signer, key)
	if err != nil {
		t.Error(err)
	}
	t.Logf("tx after sign  %v\n", signTx)

	enc, err := rlp.EncodeToBytes(&signTx.data)
	if err != nil {
		t.Logf("rlp encode to byte err %v\n", err)
	}

	fmt.Printf("ecn %x\n", enc)
}

func TestDecodeTx(t *testing.T) {
	//_, addr := defaultTestKey()

	//str := "f89c808502540be40082c350a0494e543346464646464646464646464646464646464646464646464646464646880de0b6b3a7640000a449339f0f494e54334346564e70547772335172796b685057694c50386e3977737943566128a036d1416bdc8e42917e078937571c2388cc84bb7ded75dbc195142e76ec614961a05091454f075fd35488a96f712dfa4108ea092dacbcf7d67d07fd2d2d882a7f89"
	str := "f8c4820127843b9aca00827530a0494e543346464646464646464646464646464646464646464646464646464646880de0b6b3a7640000b84a307834393333396630663439346535343333343634633533363637323539353637393738363536353461356134323736366634653633346136393464376134313632363236333332363928a0fdbfa3a7791134e739729bc2fa44260b2e9c2c01fdad959495e0e74e4c4d4d23a06a9c1af504dc8255f1b00170d5fcca9e962ea8f8fde93cb5a2c84b7bd144bcfd"

	tx, err := decodeTx(common.Hex2Bytes(str))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	fmt.Printf("decode tx %v\n", tx.String())

	//from, err := Sender(HomesteadSigner{}, tx)
	//if err != nil {
	//	t.Error(err)
	//	t.FailNow()
	//}

	//if addr != from {
	//	t.Error("derived address doesn't match")
	//}
}
