package neatcli

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"math/rand"
	"time"

	"github.com/nio-net/nio/chain/core/types"
	"github.com/nio-net/nio/chain/log"
	neatAbi "github.com/nio-net/nio/neatabi/abi"
	"github.com/nio-net/nio/params"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/common/hexutil"
	"github.com/nio-net/nio/utilities/crypto"
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

func (ec *Client) SendDataToMainChain(ctx context.Context, data []byte, prv *ecdsa.PrivateKey, mainChainId string) (common.Hash, error) {

	bs, err := neatAbi.ChainABI.Pack(neatAbi.SaveDataToMainChain.String(), data)
	if err != nil {
		return common.Hash{}, err
	}

	account := crypto.PubkeyToAddress(prv.PublicKey)

	nonce, err := ec.NonceAt(ctx, account, nil)
	if err != nil {
		return common.Hash{}, err
	}

	digest := crypto.Keccak256([]byte(mainChainId))
	signer := types.NewEIP155Signer(new(big.Int).SetBytes(digest[:]))

	var hash = common.Hash{}

	err = retry(30, time.Second*3, func() error {

		gasPrice, err := ec.SuggestGasPrice(ctx)
		if err != nil {
			return err
		}

	SendTX:

		tx := types.NewTransaction(nonce, neatAbi.ChainContractMagicAddr, nil, 0, gasPrice, bs)

		signedTx, err := types.SignTx(tx, signer, prv)
		if err != nil {
			return err
		}

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

			jitter := time.Duration(rand.Int63n(int64(sleep)))
			sleep = sleep + jitter/2

			time.Sleep(sleep)
			return retry(attemps, sleep*2, fn)
		}

		return err
	}

	return nil
}
