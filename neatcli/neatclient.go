package neatcli

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"math/rand"
	"time"

	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/common/hexutil"
	"github.com/neatlab/neatio/core/types"
	"github.com/neatlab/neatio/crypto"
	"github.com/neatlab/neatio/log"
	neatabi "github.com/neatlab/neatio/neatabi/abi"
	"github.com/neatlab/neatio/params"
	"github.com/pkg/errors"
)

func (ec *Client) BlockNumber(ctx context.Context) (*big.Int, error) {

	var hex hexutil.Big

	err := ec.c.CallContext(ctx, &hex, "eth_blockNumber")
	if err != nil {
		return nil, err
	}
	return (*big.Int)(&hex), nil
}

// SendDataToMainChain send epoch data to main chain through eth_sendRawTransaction
func (ec *Client) SendDataToMainChain(ctx context.Context, data []byte, prv *ecdsa.PrivateKey, mainChainId string) (common.Hash, error) {

	// data
	bs, err := neatabi.ChainABI.Pack(neatabi.SaveDataToMainChain.String(), data)
	if err != nil {
		return common.Hash{}, err
	}

	account := crypto.PubkeyToAddress(prv.PublicKey)

	// nonce, fetch the nonce first, if we get nonce too low error, we will manually add the value until the error gone
	nonce, err := ec.NonceAt(ctx, account, nil)
	if err != nil {
		return common.Hash{}, err
	}

	// tx signer for the main chain
	digest := crypto.Keccak256([]byte(mainChainId))
	signer := types.NewEIP155Signer(new(big.Int).SetBytes(digest[:]))

	var hash = common.Hash{}
	//should send successfully, let's wait longer time
	err = retry(30, time.Second*3, func() error {
		// gasPrice
		gasPrice, err := ec.SuggestGasPrice(ctx)
		if err != nil {
			return err
		}

	SendTX:
		// tx
		tx := types.NewTransaction(nonce, neatabi.ChainContractMagicAddr, nil, 0, gasPrice, bs)

		// sign the tx
		signedTx, err := types.SignTx(tx, signer, prv)
		if err != nil {
			return err
		}

		// eth_sendRawTransaction
		err = ec.SendTransaction(ctx, signedTx)
		if err != nil {
			if err.Error() == "nonce too low" {
				log.Warnf("SendDataToMainChain: failed, nonce too low, %v current nonce is %v. Will try to increase the nonce then send again.", account, nonce)
				nonce += 1
				goto SendTX
			} else {
				return err
			}
		}

		hash = signedTx.Hash()
		return nil
	})

	return hash, err
}

// BroadcastDataToMainChain send tx3 proof data to MainChain via rpc call, then broadcast it via p2p network
func (ec *Client) BroadcastDataToMainChain(ctx context.Context, chainId string, data []byte) error {
	if chainId == "" || chainId == params.MainnetChainConfig.NeatChainId || chainId == params.TestnetChainConfig.NeatChainId {
		return errors.New("invalid side chainId")
	}

	err := retry(1, time.Millisecond*200, func() error {
		return ec.c.CallContext(ctx, nil, "chain_broadcastTX3ProofData", common.ToHex(data))
	})

	return err
}

func retry(attemps int, sleep time.Duration, fn func() error) error {

	if err := fn(); err != nil {
		if attemps--; attemps >= 0 {
			// Add some randomness to prevent creating a Thundering Herd
			jitter := time.Duration(rand.Int63n(int64(sleep)))
			sleep = sleep + jitter/2

			time.Sleep(sleep)
			return retry(attemps, sleep*2, fn)
		}

		return err
	}

	return nil
}
