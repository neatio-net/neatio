package types

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/nio-net/bls"
	. "github.com/nio-net/common"
	"github.com/nio-net/crypto"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/wire"
)

type PrivValidator struct {
	Address common.Address `json:"address"`
	PubKey  crypto.PubKey  `json:"consensus_pub_key"`
	PrivKey crypto.PrivKey `json:"consensus_priv_key"`

	Signer `json:"-"`

	filePath string
	mtx      sync.Mutex
}

type PrivV struct {
	Address string         `json:"address"`
	PubKey  crypto.PubKey  `json:"consensus_pub_key"`
	PrivKey crypto.PrivKey `json:"consensus_priv_key"`

	Signer `json:"-"`

	filePath string
	mtx      sync.Mutex
}

type Signer interface {
	Sign(msg []byte) crypto.Signature
}

type DefaultSigner struct {
	priv crypto.PrivKey
}

func NewDefaultSigner(priv crypto.PrivKey) *DefaultSigner {
	return &DefaultSigner{priv: priv}
}

func (ds *DefaultSigner) Sign(msg []byte) crypto.Signature {
	return ds.priv.Sign(msg)
}

func GenPrivValidatorKey(address common.Address) *PrivValidator {

	keyPair := bls.GenerateKey()
	var blsPrivKey crypto.BLSPrivKey
	copy(blsPrivKey[:], keyPair.Private().Marshal())

	blsPubKey := blsPrivKey.PubKey()

	return &PrivValidator{
		Address: address,
		PubKey:  blsPubKey,
		PrivKey: blsPrivKey,

		filePath: "",
		Signer:   NewDefaultSigner(blsPrivKey),
	}
}

func LoadPrivValidator(filePath string) *PrivValidator {
	privValJSONBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		Exit(err.Error())
	}
	privVal := wire.ReadJSON(&PrivV{}, privValJSONBytes, &err).(*PrivV)

	if err != nil {
		Exit(Fmt("Error reading PrivValidator from %v: %v\n", filePath, err))
	}
	privV := &PrivValidator{
		Address:  common.StringToAddress(privVal.Address),
		PubKey:   privVal.PubKey,
		PrivKey:  privVal.PrivKey,
		filePath: filePath,
		Signer:   NewDefaultSigner(privVal.PrivKey),
	}

	return privV
}

func (pv *PrivValidator) SetFile(filePath string) {
	pv.mtx.Lock()
	defer pv.mtx.Unlock()

	pv.filePath = filePath
}

func (pv *PrivValidator) Save() {
	pv.mtx.Lock()
	defer pv.mtx.Unlock()

	pv.save()
}

func (pv *PrivValidator) save() {
	if pv.filePath == "" {
		PanicSanity("Cannot save PrivValidator: filePath not set")
	}
	var priv PrivV
	priv.Address = pv.Address.String()
	priv.PubKey = pv.PubKey
	priv.PrivKey = pv.PrivKey

	jsonBytes := wire.JSONBytesPretty(priv)
	err := WriteFileAtomic(pv.filePath, jsonBytes, 0600)
	if err != nil {
		PanicCrisis(err)
	}
}

func (pv *PrivValidator) GetAddress() []byte {
	return pv.Address.Bytes()
}

func (pv *PrivValidator) GetPubKey() crypto.PubKey {
	return pv.PubKey
}

func (pv *PrivValidator) SignVote(chainID string, vote *Vote) error {
	pv.mtx.Lock()
	defer pv.mtx.Unlock()

	signature := pv.Sign(SignBytes(chainID, vote))
	vote.Signature = signature
	return nil
}

func (pv *PrivValidator) SignProposal(chainID string, proposal *Proposal) error {
	pv.mtx.Lock()
	defer pv.mtx.Unlock()

	signature := pv.Sign(SignBytes(chainID, proposal))
	proposal.Signature = signature
	return nil
}

func (pv *PrivValidator) String() string {
	return fmt.Sprintf("PrivValidator{%X}", pv.Address)
}

type PrivValidatorsByAddress []*PrivValidator

func (pvs PrivValidatorsByAddress) Len() int {
	return len(pvs)
}

func (pvs PrivValidatorsByAddress) Less(i, j int) bool {
	return bytes.Compare(pvs[i].Address[:], pvs[j].Address[:]) == -1
}

func (pvs PrivValidatorsByAddress) Swap(i, j int) {
	it := pvs[i]
	pvs[i] = pvs[j]
	pvs[j] = it
}
