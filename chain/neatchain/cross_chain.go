package main

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Gessiux/go-crypto"
	dbm "github.com/Gessiux/go-db"
	"github.com/neatlab/neatio/chain/consensus"
	"github.com/neatlab/neatio/chain/consensus/neatcon/epoch"
	ntcTypes "github.com/neatlab/neatio/chain/consensus/neatcon/types"
	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/chain/core/rawdb"
	"github.com/neatlab/neatio/chain/core/state"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/chain/trie"
	neatAbi "github.com/neatlab/neatio/neatabi/abi"
	"github.com/neatlab/neatio/neatcli"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/neatptc"
	"github.com/neatlab/neatio/network/node"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/math"
	"github.com/neatlab/neatio/utilities/rlp"
)

type CrossChainHelper struct {
	mtx             sync.Mutex
	chainInfoDB     dbm.DB
	localTX3CacheDB neatdb.Database
	//the client does only connect to main chain
	client      *neatcli.Client
	mainChainId string
}

func (cch *CrossChainHelper) GetMutex() *sync.Mutex {
	return &cch.mtx
}

func (cch *CrossChainHelper) GetChainInfoDB() dbm.DB {
	return cch.chainInfoDB
}

func (cch *CrossChainHelper) GetClient() *neatcli.Client {
	return cch.client
}

func (cch *CrossChainHelper) GetMainChainId() string {
	return cch.mainChainId
}

// CanCreateSideChain check the condition before send the create side chain into the tx pool
func (cch *CrossChainHelper) CanCreateSideChain(from common.Address, chainId string, minValidators uint16, minDepositAmount, startupCost *big.Int, startBlock, endBlock *big.Int) error {

	if chainId == "" || strings.Contains(chainId, ";") {
		return errors.New("chainId is nil or empty, or contains ';', should be meaningful")
	}

	pass, _ := regexp.MatchString("^[a-z]+[a-z0-9_]*$", chainId)
	if !pass {
		return errors.New("chainId must be start with letter (a-z) and contains alphanumeric(lower case) or underscore, try use other name instead")
	}

	if utf8.RuneCountInString(chainId) > 30 {
		return errors.New("max characters of chain id is 30, try use other name instead")
	}

	if chainId == MainChain || chainId == TestnetChain {
		return errors.New("you can't create NeatIO as a side chain, try use other name instead")
	}

	// Check if "chainId" has been created
	ci := core.GetChainInfo(cch.chainInfoDB, chainId)
	if ci != nil {
		return fmt.Errorf("Chain %s has already exist, try use other name instead", chainId)
	}

	// Check if "chainId" has been registered
	cci := core.GetPendingSideChainData(cch.chainInfoDB, chainId)
	if cci != nil {
		return fmt.Errorf("Chain %s has already applied, try use other name instead", chainId)
	}

	// Check the minimum validators
	if minValidators < core.OFFICIAL_MINIMUM_VALIDATORS {
		return fmt.Errorf("Validators count is not meet the minimum official validator count (%v)", core.OFFICIAL_MINIMUM_VALIDATORS)
	}

	// Check the minimum deposit amount
	officialMinimumDeposit := math.MustParseBig256(core.OFFICIAL_MINIMUM_DEPOSIT)
	if minDepositAmount.Cmp(officialMinimumDeposit) == -1 {
		return fmt.Errorf("Deposit amount is not meet the minimum official deposit amount (%v PI)", new(big.Int).Div(officialMinimumDeposit, big.NewInt(params.NEAT)))
	}

	// Check the startup cost
	if startupCost.Cmp(officialMinimumDeposit) != 0 {
		return fmt.Errorf("Startup cost is not meet the required amount (%v PI)", new(big.Int).Div(officialMinimumDeposit, big.NewInt(params.NEAT)))
	}

	// Check start/end block
	if startBlock.Cmp(endBlock) >= 0 {
		return errors.New("start block number must be less than end block number")
	}

	// Check End Block already passed
	neatio := MustGetNeatChainFromNode(chainMgr.mainChain.NeatNode)
	currentBlock := neatio.BlockChain().CurrentBlock()
	if endBlock.Cmp(currentBlock.Number()) <= 0 {
		return errors.New("end block number has already passed")
	}

	return nil
}

// CreateSideChain Save the Side Chain Data into the DB, the data will be used later during Block Commit Callback
func (cch *CrossChainHelper) CreateSideChain(from common.Address, chainId string, minValidators uint16, minDepositAmount *big.Int, startBlock, endBlock *big.Int) error {
	log.Debug("CreateSideChain - start")

	cci := &core.CoreChainInfo{
		Owner:            from,
		ChainId:          chainId,
		MinValidators:    minValidators,
		MinDepositAmount: minDepositAmount,
		StartBlock:       startBlock,
		EndBlock:         endBlock,
		JoinedValidators: make([]core.JoinedValidator, 0),
	}
	core.CreatePendingSideChainData(cch.chainInfoDB, cci)

	log.Debug("CreateSideChain - end")
	return nil
}

// ValidateJoinSideChain check the criteria whether it meets the join side chain requirement
func (cch *CrossChainHelper) ValidateJoinSideChain(from common.Address, consensusPubkey []byte, chainId string, depositAmount *big.Int, signature []byte) error {
	log.Debug("ValidateJoinSideChain - start")

	if chainId == MainChain || chainId == TestnetChain {
		return errors.New("you can't join NeatIO as a side chain, try use other name instead")
	}

	// Check Signature of the PubKey matched against the Address
	if err := crypto.CheckConsensusPubKey(from, consensusPubkey, signature); err != nil {
		return err
	}

	// Check if "chainId" has been created/registered
	ci := core.GetPendingSideChainData(cch.chainInfoDB, chainId)
	if ci == nil {
		if core.GetChainInfo(cch.chainInfoDB, chainId) != nil {
			return fmt.Errorf("chain %s has already created/started, try use other name instead", chainId)
		} else {
			return fmt.Errorf("side chain %s not exist, try use other name instead", chainId)
		}
	}

	// Check if already joined the chain
	find := false
	for _, joined := range ci.JoinedValidators {
		if from == joined.Address {
			find = true
			break
		}
	}

	if find {
		return errors.New(fmt.Sprintf("You have already joined the Side Chain %s", chainId))
	}

	// Check the deposit amount
	if !(depositAmount != nil && depositAmount.Sign() == 1) {
		return errors.New("deposit amount must be greater than 0")
	}

	log.Debug("ValidateJoinSideChain - end")
	return nil
}

// JoinSideChain Join the Side Chain
func (cch *CrossChainHelper) JoinSideChain(from common.Address, pubkey crypto.PubKey, chainId string, depositAmount *big.Int) error {
	log.Debug("JoinSideChain - start")

	// Load the Side Chain first
	ci := core.GetPendingSideChainData(cch.chainInfoDB, chainId)
	if ci == nil {
		log.Errorf("JoinSideChain - Side Chain %s not exist, you can't join the chain", chainId)
		return fmt.Errorf("Side Chain %s not exist, you can't join the chain", chainId)
	}

	for _, joined := range ci.JoinedValidators {
		if from == joined.Address {
			return nil
		}
	}

	jv := core.JoinedValidator{
		PubKey:        pubkey,
		Address:       from,
		DepositAmount: depositAmount,
	}

	ci.JoinedValidators = append(ci.JoinedValidators, jv)

	core.UpdatePendingSideChainData(cch.chainInfoDB, ci)

	log.Debug("JoinSideChain - end")
	return nil
}

func (cch *CrossChainHelper) ReadyForLaunchSideChain(height *big.Int, stateDB *state.StateDB) ([]string, []byte, []string) {
	//log.Debug("ReadyForLaunchSideChain - start")

	readyId, updateBytes, removedId := core.GetSideChainForLaunch(cch.chainInfoDB, height, stateDB)
	if len(readyId) == 0 {
		//log.Debugf("ReadyForLaunchSideChain - No side chain to be launch in Block %v", height)
	} else {
		//log.Infof("ReadyForLaunchSideChain - %v side chain(s) to be launch in Block %v. %v", len(readyId), height, readyId)
	}

	//log.Debug("ReadyForLaunchSideChain - end")
	return readyId, updateBytes, removedId
}

func (cch *CrossChainHelper) ProcessPostPendingData(newPendingIdxBytes []byte, deleteSideChainIds []string) {
	core.ProcessPostPendingData(cch.chainInfoDB, newPendingIdxBytes, deleteSideChainIds)
}

func (cch *CrossChainHelper) VoteNextEpoch(ep *epoch.Epoch, from common.Address, voteHash common.Hash, txHash common.Hash) error {

	voteSet := ep.GetNextEpoch().GetEpochValidatorVoteSet()
	if voteSet == nil {
		voteSet = epoch.NewEpochValidatorVoteSet()
	}

	vote, exist := voteSet.GetVoteByAddress(from)

	if exist {
		// Overwrite the Previous Hash Vote
		vote.VoteHash = voteHash
		vote.TxHash = txHash
	} else {
		// Create a new Hash Vote
		vote = &epoch.EpochValidatorVote{
			Address:  from,
			VoteHash: voteHash,
			TxHash:   txHash,
		}
		voteSet.StoreVote(vote)
	}
	// Save the VoteSet
	epoch.SaveEpochVoteSet(ep.GetDB(), ep.GetNextEpoch().Number, voteSet)
	return nil
}

func (cch *CrossChainHelper) RevealVote(ep *epoch.Epoch, from common.Address, pubkey crypto.PubKey, depositAmount *big.Int, salt string, txHash common.Hash) error {

	voteSet := ep.GetNextEpoch().GetEpochValidatorVoteSet()
	vote, exist := voteSet.GetVoteByAddress(from)

	if exist {
		// Update the Hash Vote with Real Data
		vote.PubKey = pubkey
		vote.Amount = depositAmount
		vote.Salt = salt
		vote.TxHash = txHash
	}
	// Save the VoteSet
	epoch.SaveEpochVoteSet(ep.GetDB(), ep.GetNextEpoch().Number, voteSet)
	return nil
}

func (cch *CrossChainHelper) UpdateNextEpoch(ep *epoch.Epoch, from common.Address, pubkey crypto.PubKey, depositAmount *big.Int, salt string, txHash common.Hash) error {
	voteSet := ep.GetNextEpoch().GetEpochValidatorVoteSet()
	if voteSet == nil {
		voteSet = epoch.NewEpochValidatorVoteSet()
	}

	vote, exist := voteSet.GetVoteByAddress(from)

	if exist {
		vote.Amount = depositAmount
		vote.TxHash = txHash
	} else {
		vote = &epoch.EpochValidatorVote{
			Address: from,
			PubKey:  pubkey,
			Amount:  depositAmount,
			Salt:    "neatio",
			TxHash:  txHash,
		}

		voteSet.StoreVote(vote)
	}

	// Save the VoteSet
	epoch.SaveEpochVoteSet(ep.GetDB(), ep.GetNextEpoch().Number, voteSet)
	return nil
}

func (cch *CrossChainHelper) GetHeightFromMainChain() *big.Int {
	neatio := MustGetNeatChainFromNode(chainMgr.mainChain.NeatNode)
	return neatio.BlockChain().CurrentBlock().Number()
}

func (cch *CrossChainHelper) GetTxFromMainChain(txHash common.Hash) *types.Transaction {
	neatio := MustGetNeatChainFromNode(chainMgr.mainChain.NeatNode)
	chainDb := neatio.ChainDb()

	tx, _, _, _ := rawdb.ReadTransaction(chainDb, txHash)
	return tx
}

func (cch *CrossChainHelper) GetEpochFromMainChain() (string, *epoch.Epoch) {
	neatio := MustGetNeatChainFromNode(chainMgr.mainChain.NeatNode)
	var ep *epoch.Epoch
	if neatcon, ok := neatio.Engine().(consensus.NeatCon); ok {
		ep = neatcon.GetEpoch()
	}
	return neatio.ChainConfig().NeatChainId, ep
}

func (cch *CrossChainHelper) ChangeValidators(chainId string) {

	if chainMgr == nil {
		return
	}

	var chain *Chain = nil
	if chainId == MainChain || chainId == TestnetChain {
		chain = chainMgr.mainChain
	} else if chn, ok := chainMgr.sideChains[chainId]; ok {
		chain = chn
	}

	if chain == nil || chain.NeatNode == nil {
		return
	}

	if address, ok := chainMgr.getNodeValidator(chain.NeatNode); ok {
		chainMgr.server.AddLocalValidator(chainId, address)
	}
}

// verify the signature of validators who voted for the block
// most of the logic here is from 'VerifyHeader'
func (cch *CrossChainHelper) VerifySideChainProofData(bs []byte) error {

	log.Debug("VerifySideChainProofData - start")

	var proofData types.SideChainProofData
	err := rlp.DecodeBytes(bs, &proofData)
	if err != nil {
		return err
	}

	header := proofData.Header
	// Don't waste time checking blocks from the future
	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
		//return errors.New("block in the future")
	}

	ncExtra, err := ntcTypes.ExtractNeatConExtra(header)
	if err != nil {
		return err
	}

	chainId := ncExtra.ChainID
	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
		return fmt.Errorf("invalid side chain id: %s", chainId)
	}

	if header.Nonce != (types.NeatConEmptyNonce) && !bytes.Equal(header.Nonce[:], types.NeatConNonce) {
		return errors.New("invalid nonce")
	}

	if header.MixDigest != types.NeatConDigest {
		return errors.New("invalid mix digest")
	}

	if header.UncleHash != types.NeatConNilUncleHash {
		return errors.New("invalid uncle Hash")
	}

	if header.Difficulty == nil || header.Difficulty.Cmp(types.NeatConDefaultDifficulty) != 0 {
		return errors.New("invalid difficulty")
	}

	// special case: epoch 0 update
	// TODO: how to verify this block which includes epoch 0?
	if ncExtra.EpochBytes != nil && len(ncExtra.EpochBytes) != 0 {
		ep := epoch.FromBytes(ncExtra.EpochBytes)
		if ep != nil && ep.Number == 0 {
			return nil
		}
	}

	// Bypass the validator check for official side chain 0
	// TODO where need
	if chainId != "side_0" {
		ci := core.GetChainInfo(cch.chainInfoDB, chainId)
		if ci == nil {
			return fmt.Errorf("chain info %s not found", chainId)
		}
		epoch := ci.GetEpochByBlockNumber(ncExtra.Height)
		if epoch == nil {
			return fmt.Errorf("could not get epoch for block height %v", ncExtra.Height)
		}
		valSet := epoch.Validators
		if !bytes.Equal(valSet.Hash(), ncExtra.ValidatorsHash) {
			return errors.New("inconsistent validator set")
		}

		seenCommit := ncExtra.SeenCommit
		if !bytes.Equal(ncExtra.SeenCommitHash, seenCommit.Hash()) {
			return errors.New("invalid committed seals")
		}

		if err = valSet.VerifyCommit(ncExtra.ChainID, ncExtra.Height, seenCommit); err != nil {
			return err
		}
	}

	log.Debug("VerifySideChainProofData - end")
	return nil
}

func (cch *CrossChainHelper) SaveSideChainProofDataToMainChain(bs []byte) error {
	log.Debug("SaveSideChainProofDataToMainChain - start")

	var proofData types.SideChainProofData
	err := rlp.DecodeBytes(bs, &proofData)
	if err != nil {
		return err
	}

	header := proofData.Header
	ncExtra, err := ntcTypes.ExtractNeatConExtra(header)
	if err != nil {
		return err
	}

	chainId := ncExtra.ChainID
	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
		return fmt.Errorf("invalid side chain id: %s", chainId)
	}

	// here is epoch update; should be a more general mechanism
	if len(ncExtra.EpochBytes) != 0 {
		ep := epoch.FromBytes(ncExtra.EpochBytes)
		if ep != nil {
			ci := core.GetChainInfo(cch.chainInfoDB, ncExtra.ChainID)
			// ChainInfo is nil means we need to wait for Side Chain to be launched, this could happened during catch-up scenario
			if ci == nil {
				for {
					// wait for 3 sec and try again
					time.Sleep(3 * time.Second)
					ci = core.GetChainInfo(cch.chainInfoDB, ncExtra.ChainID)
					if ci != nil {
						break
					}
				}
			}

			futureEpoch := ep.Number > ci.EpochNumber && ncExtra.Height < ep.StartBlock
			if futureEpoch {
				// Future Epoch, just save the Epoch into Chain Info DB
				core.SaveFutureEpoch(cch.chainInfoDB, ep, chainId)
				log.Infof("Future epoch saved from chain: %s, epoch: %v", chainId, ep)
			} else if ep.Number == 0 || ep.Number >= ci.EpochNumber {
				// New Epoch, save or update the Epoch into Chain Info DB
				ci.EpochNumber = ep.Number
				ci.Epoch = ep
				core.SaveChainInfo(cch.chainInfoDB, ci)
				log.Infof("Epoch saved from chain: %s, epoch: %v", chainId, ep)
			}
		}
	}

	log.Debug("SaveSideChainProofDataToMainChain - end")
	return nil
}

func (cch *CrossChainHelper) ValidateTX3ProofData(proofData *types.TX3ProofData) error {
	log.Debug("ValidateTX3ProofData - start")

	header := proofData.Header
	// Don't waste time checking blocks from the future
	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
		//return errors.New("block in the future")
	}

	ncExtra, err := ntcTypes.ExtractNeatConExtra(header)
	if err != nil {
		return err
	}

	chainId := ncExtra.ChainID
	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
		return fmt.Errorf("invalid side chain id: %s", chainId)
	}

	if header.Nonce != (types.NeatConEmptyNonce) && !bytes.Equal(header.Nonce[:], types.NeatConNonce) {
		return errors.New("invalid nonce")
	}

	if header.MixDigest != types.NeatConDigest {
		return errors.New("invalid mix digest")
	}

	if header.UncleHash != types.NeatConNilUncleHash {
		return errors.New("invalid uncle Hash")
	}

	if header.Difficulty == nil || header.Difficulty.Cmp(types.NeatConDefaultDifficulty) != 0 {
		return errors.New("invalid difficulty")
	}

	// special case: epoch 0 update
	// TODO: how to verify this block which includes epoch 0?
	if ncExtra.EpochBytes != nil && len(ncExtra.EpochBytes) != 0 {
		ep := epoch.FromBytes(ncExtra.EpochBytes)
		if ep != nil && ep.Number == 0 {
			return nil
		}
	}

	ci := core.GetChainInfo(cch.chainInfoDB, chainId)
	if ci == nil {
		return fmt.Errorf("chain info %s not found", chainId)
	}
	epoch := ci.GetEpochByBlockNumber(ncExtra.Height)
	if epoch == nil {
		return fmt.Errorf("could not get epoch for block height %v", ncExtra.Height)
	}
	valSet := epoch.Validators
	if !bytes.Equal(valSet.Hash(), ncExtra.ValidatorsHash) {
		return errors.New("inconsistent validator set")
	}

	seenCommit := ncExtra.SeenCommit
	if !bytes.Equal(ncExtra.SeenCommitHash, seenCommit.Hash()) {
		return errors.New("invalid committed seals")
	}

	if err = valSet.VerifyCommit(ncExtra.ChainID, ncExtra.Height, seenCommit); err != nil {
		return err
	}

	// tx merkle proof verify
	keybuf := new(bytes.Buffer)
	for i, txIndex := range proofData.TxIndexs {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(txIndex))
		_, _, err := trie.VerifyProof(header.TxHash, keybuf.Bytes(), proofData.TxProofs[i])
		if err != nil {
			return err
		}
	}

	log.Debug("ValidateTX3ProofData - end")
	return nil
}

func (cch *CrossChainHelper) ValidateTX4WithInMemTX3ProofData(tx4 *types.Transaction, tx3ProofData *types.TX3ProofData) error {
	// TX4
	signer := types.NewEIP155Signer(tx4.ChainId())
	from, err := types.Sender(signer, tx4)
	if err != nil {
		return core.ErrInvalidSender
	}

	var args neatAbi.WithdrawFromMainChainArgs

	if !neatAbi.IsNeatChainContractAddr(tx4.To()) {
		return errors.New("invalid TX4: wrong To()")
	}

	data := tx4.Data()
	function, err := neatAbi.FunctionTypeFromId(data[:4])
	if err != nil {
		return err
	}

	if function != neatAbi.WithdrawFromMainChain {
		return errors.New("invalid TX4: wrong function")
	}

	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.WithdrawFromMainChain.String(), data[4:]); err != nil {
		return err
	}

	// TX3
	header := tx3ProofData.Header
	if err != nil {
		return err
	}
	keybuf := new(bytes.Buffer)
	rlp.Encode(keybuf, tx3ProofData.TxIndexs[0])
	val, _, err := trie.VerifyProof(header.TxHash, keybuf.Bytes(), tx3ProofData.TxProofs[0])
	if err != nil {
		return err
	}

	var tx3 types.Transaction
	err = rlp.DecodeBytes(val, &tx3)
	if err != nil {
		return err
	}

	signer2 := types.NewEIP155Signer(tx3.ChainId())
	tx3From, err := types.Sender(signer2, &tx3)
	if err != nil {
		return core.ErrInvalidSender
	}

	var tx3Args neatAbi.WithdrawFromSideChainArgs
	tx3Data := tx3.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&tx3Args, neatAbi.WithdrawFromSideChain.String(), tx3Data[4:]); err != nil {
		return err
	}

	// Does TX3 & TX4 Match
	if from != tx3From || args.ChainId != tx3Args.ChainId || args.Amount.Cmp(tx3.Value()) != 0 {
		return errors.New("params are not consistent with tx in side chain")
	}

	return nil
}

//SaveDataToMainV1 acceps both epoch and tx3
//func (cch *CrossChainHelper) VerifySideChainProofDataV1(proofData *types.SideChainProofDataV1) error {
//
//	log.Debug("VerifySideChainProofDataV1 - start")
//
//	header := proofData.Header
//	// Don't waste time checking blocks from the future
//	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
//		//return errors.New("block in the future")
//	}
//
//	ncExtra, err := ntcTypes.ExtractNeatConExtra(header)
//	if err != nil {
//		return err
//	}
//
//	chainId := ncExtra.ChainID
//	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
//		return fmt.Errorf("invalid side chain id: %s", chainId)
//	}
//
//	if header.Nonce != (types.NeatConEmptyNonce) && !bytes.Equal(header.Nonce[:], types.NeatConNonce) {
//		return errors.New("invalid nonce")
//	}
//
//	if header.MixDigest != types.NeatConDigest {
//		return errors.New("invalid mix digest")
//	}
//
//	if header.UncleHash != types.NeatConNilUncleHash {
//		return errors.New("invalid uncle Hash")
//	}
//
//	if header.Difficulty == nil || header.Difficulty.Cmp(types.NeatConDefaultDifficulty) != 0 {
//		return errors.New("invalid difficulty")
//	}
//
//	ci := core.GetChainInfo(cch.chainInfoDB, chainId)
//	if ci == nil {
//		return fmt.Errorf("chain info %s not found", chainId)
//	}
//
//	isSd2mc := params.IsSd2mc(cch.GetMainChainId(), cch.GetHeightFromMainChain())
//	// Bypass the validator check for official side chain 0
//	if chainId != "side_0" || isSd2mc {
//
//		getValidatorsFromChainInfo := false
//		if ncExtra.EpochBytes != nil && len(ncExtra.EpochBytes) != 0 {
//			ep := epoch.FromBytes(ncExtra.EpochBytes)
//			if ep != nil && ep.Number == 0 {
//				//Side chain just created and save the epoch info, get validators from chain info
//				getValidatorsFromChainInfo = true
//			}
//		}
//
//		var valSet *ntcTypes.ValidatorSet = nil
//		if !getValidatorsFromChainInfo {
//			ep := ci.GetEpochByBlockNumber(ncExtra.Height)
//			if ep == nil {
//				return fmt.Errorf("could not get epoch for block height %v", ncExtra.Height)
//			}
//			valSet = ep.Validators
//		} else {
//			_, ntcGenesis := core.LoadChainGenesis(cch.chainInfoDB, chainId)
//			if ntcGenesis == nil {
//				return errors.New(fmt.Sprintf("unable to retrieve the genesis file for side chain %s", chainId))
//			}
//			coreGenesis, err := ntcTypes.GenesisDocFromJSON(ntcGenesis)
//			if err != nil {
//				return err
//			}
//
//			ep := epoch.MakeOneEpoch(nil, &coreGenesis.CurrentEpoch, nil)
//			if ep == nil {
//				return fmt.Errorf("could not get epoch for genesis information")
//			}
//			valSet = ep.Validators
//		}
//
//		if !bytes.Equal(valSet.Hash(), ncExtra.ValidatorsHash) {
//			return errors.New("inconsistent validator set")
//		}
//
//		seenCommit := ncExtra.SeenCommit
//		if !bytes.Equal(ncExtra.SeenCommitHash, seenCommit.Hash()) {
//			return errors.New("invalid committed seals")
//		}
//
//		if err = valSet.VerifyCommit(ncExtra.ChainID, ncExtra.Height, seenCommit); err != nil {
//			return err
//		}
//	}
//
//	//Verify Tx3
//	// tx merkle proof verify
//	keybuf := new(bytes.Buffer)
//	for i, txIndex := range proofData.TxIndexs {
//		keybuf.Reset()
//		rlp.Encode(keybuf, uint(txIndex))
//		_, _, err := trie.VerifyProof(header.TxHash, keybuf.Bytes(), proofData.TxProofs[i])
//		if err != nil {
//			return err
//		}
//	}
//
//	log.Debug("VerifySideChainProofDataV1 - end")
//	return nil
//}
//
//func (cch *CrossChainHelper) SaveSideChainProofDataToMainChainV1(proofData *types.SideChainProofDataV1) error {
//	log.Info("SaveSideChainProofDataToMainChainV1 - start")
//
//	header := proofData.Header
//	ncExtra, err := ntcTypes.ExtractNeatConExtra(header)
//	if err != nil {
//		return err
//	}
//
//	chainId := ncExtra.ChainID
//	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
//		return fmt.Errorf("invalid side chain id: %s", chainId)
//	}
//
//	// here is epoch update; should be a more general mechanism
//	if len(ncExtra.EpochBytes) != 0 {
//		log.Info("SaveSideChainProofDataToMainChainV1 - Save Epoch")
//		ep := epoch.FromBytes(ncExtra.EpochBytes)
//		if ep != nil {
//			ci := core.GetChainInfo(cch.chainInfoDB, ncExtra.ChainID)
//			// ChainInfo is nil means we need to wait for Side Chain to be launched, this could happened during catch-up scenario
//			if ci == nil {
//				return fmt.Errorf("not possible to pass verification")
//			}
//
//			futureEpoch := ep.Number > ci.EpochNumber && ncExtra.Height < ep.StartBlock
//			if futureEpoch {
//				// Future Epoch, just save the Epoch into Chain Info DB
//				core.SaveFutureEpoch(cch.chainInfoDB, ep, chainId)
//			} else if ep.Number == 0 || ep.Number >= ci.EpochNumber {
//				// New Epoch, save or update the Epoch into Chain Info DB
//				ci.EpochNumber = ep.Number
//				ci.Epoch = ep
//				core.SaveChainInfo(cch.chainInfoDB, ci)
//				log.Infof("Epoch saved from chain: %s, epoch: %v", chainId, ep)
//			}
//		}
//	}
//
//	// Write the TX3ProofData
//	if len(proofData.TxIndexs) != 0 {
//
//		log.Infof("SaveSideChainProofDataToMainChainV1 - Save Tx3, count is %v", len(proofData.TxIndexs))
//		tx3ProofData := &types.TX3ProofData{
//			Header:   proofData.Header,
//			TxIndexs: proofData.TxIndexs,
//			TxProofs: proofData.TxProofs,
//		}
//		if err := cch.WriteTX3ProofData(tx3ProofData); err != nil {
//			log.Error("TX3ProofDataMsg write error", "error", err)
//		}
//	}
//
//	log.Info("SaveSideChainProofDataToMainChainV1 - end")
//	return nil
//}

// TX3LocalCache start
func (cch *CrossChainHelper) GetTX3(chainId string, txHash common.Hash) *types.Transaction {
	return rawdb.GetTX3(cch.localTX3CacheDB, chainId, txHash)
}

func (cch *CrossChainHelper) DeleteTX3(chainId string, txHash common.Hash) {
	rawdb.DeleteTX3(cch.localTX3CacheDB, chainId, txHash)
}

func (cch *CrossChainHelper) WriteTX3ProofData(proofData *types.TX3ProofData) error {
	return rawdb.WriteTX3ProofData(cch.localTX3CacheDB, proofData)
}

func (cch *CrossChainHelper) GetTX3ProofData(chainId string, txHash common.Hash) *types.TX3ProofData {
	return rawdb.GetTX3ProofData(cch.localTX3CacheDB, chainId, txHash)
}

func (cch *CrossChainHelper) GetAllTX3ProofData() []*types.TX3ProofData {
	return rawdb.GetAllTX3ProofData(cch.localTX3CacheDB)
}

// TX3LocalCache end

func MustGetNeatChainFromNode(node *node.Node) *neatptc.NeatIO {
	neatChain, err := getNeatChainFromNode(node)
	if err != nil {
		panic("getNeatChainFromNode error: " + err.Error())
	}
	return neatChain
}

func getNeatChainFromNode(node *node.Node) (*neatptc.NeatIO, error) {
	var neatChain *neatptc.NeatIO
	if err := node.Service(&neatChain); err != nil {
		return nil, err
	}

	return neatChain, nil
}
