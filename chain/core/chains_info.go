package core

import (
	"bytes"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"

	"github.com/nio-net/crypto"
	dbm "github.com/nio-net/database"
	ep "github.com/nio-net/nio/chain/consensus/neatcon/epoch"
	"github.com/nio-net/nio/chain/core/state"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/common/math"
	"github.com/nio-net/wire"
)

const (
	OFFICIAL_MINIMUM_VALIDATORS = 1
	OFFICIAL_MINIMUM_DEPOSIT    = "77000000000000000000000"
)

type CoreChainInfo struct {
	db dbm.DB

	Owner   common.Address
	ChainId string

	MinValidators    uint16
	MinDepositAmount *big.Int
	StartBlock       *big.Int
	EndBlock         *big.Int

	JoinedValidators []JoinedValidator

	EpochNumber uint64

	DepositInMainChain    *big.Int
	DepositInSideChain    *big.Int
	WithdrawFromSideChain *big.Int
	WithdrawFromMainChain *big.Int
}

type JoinedValidator struct {
	PubKey        crypto.PubKey
	Address       common.Address
	DepositAmount *big.Int
}

type ChainInfo struct {
	CoreChainInfo

	Epoch *ep.Epoch
}

const (
	chainInfoKey  = "CHAIN"
	ethGenesisKey = "ETH_GENESIS"
	ntcGenesisKey = "NTC_GENESIS"
)

var allChainKey = []byte("AllChainID")

const specialSep = ";"

var mtx sync.RWMutex

func calcCoreChainInfoKey(chainId string) []byte {
	return []byte(chainInfoKey + ":" + chainId)
}

func calcEpochKey(number uint64, chainId string) []byte {
	return []byte(chainInfoKey + fmt.Sprintf("-%v-%s", number, chainId))
}

func calcETHGenesisKey(chainId string) []byte {
	return []byte(ethGenesisKey + ":" + chainId)
}

func calcNTCGenesisKey(chainId string) []byte {
	return []byte(ntcGenesisKey + ":" + chainId)
}

func GetChainInfo(db dbm.DB, chainId string) *ChainInfo {
	mtx.RLock()
	defer mtx.RUnlock()

	cci := loadCoreChainInfo(db, chainId)
	if cci == nil {
		return nil
	}

	ci := &ChainInfo{
		CoreChainInfo: *cci,
	}

	epoch := loadEpoch(db, cci.EpochNumber, chainId)
	if epoch != nil {
		ci.Epoch = epoch
	}

	log.Debugf("LoadChainInfo(), chainInfo is: %v\n", ci)

	return ci
}

func SaveChainInfo(db dbm.DB, ci *ChainInfo) error {
	mtx.Lock()
	defer mtx.Unlock()

	log.Debugf("ChainInfo Save(), info is: (%v)\n", ci)

	err := saveCoreChainInfo(db, &ci.CoreChainInfo)
	if err != nil {
		return err
	}

	if ci.Epoch != nil {
		err = saveEpoch(db, ci.Epoch, ci.ChainId)
		if err != nil {
			return err
		}
	}

	saveId(db, ci.ChainId)

	return nil
}

func SaveFutureEpoch(db dbm.DB, futureEpoch *ep.Epoch, chainId string) error {
	mtx.Lock()
	defer mtx.Unlock()

	if futureEpoch != nil {
		err := saveEpoch(db, futureEpoch, chainId)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadCoreChainInfo(db dbm.DB, chainId string) *CoreChainInfo {

	cci := CoreChainInfo{db: db}
	buf := db.Get(calcCoreChainInfoKey(chainId))
	if len(buf) == 0 {
		return nil
	} else {
		r, n, err := bytes.NewReader(buf), new(int), new(error)
		wire.ReadBinaryPtr(&cci, r, 0, n, err)
		if *err != nil {

			log.Debugf("LoadChainInfo: Data has been corrupted or its spec has changed: %v", *err)
			os.Exit(1)
		}
	}
	return &cci
}

func saveCoreChainInfo(db dbm.DB, cci *CoreChainInfo) error {

	db.SetSync(calcCoreChainInfoKey(cci.ChainId), wire.BinaryBytes(*cci))
	return nil
}

func (cci *CoreChainInfo) TotalDeposit() *big.Int {
	sum := big.NewInt(0)
	for _, v := range cci.JoinedValidators {
		sum.Add(sum, v.DepositAmount)
	}
	return sum
}

func loadEpoch(db dbm.DB, number uint64, chainId string) *ep.Epoch {
	epochBytes := db.Get(calcEpochKey(number, chainId))
	return ep.FromBytes(epochBytes)
}

func saveEpoch(db dbm.DB, epoch *ep.Epoch, chainId string) error {

	db.SetSync(calcEpochKey(epoch.Number, chainId), epoch.Bytes())
	return nil
}

func (ci *ChainInfo) GetEpochByBlockNumber(blockNumber uint64) *ep.Epoch {
	mtx.RLock()
	defer mtx.RUnlock()

	if blockNumber < 0 {
		return ci.Epoch
	} else {
		epoch := ci.Epoch
		if epoch == nil {
			return nil
		}
		if blockNumber >= epoch.StartBlock && blockNumber <= epoch.EndBlock {
			return epoch
		}

		if blockNumber > epoch.EndBlock {
			ep := loadEpoch(ci.db, epoch.Number+1, ci.ChainId)
			return ep
		}

		number := epoch.Number
		for {
			if number == 0 {
				break
			}
			number--

			ep := loadEpoch(ci.db, number, ci.ChainId)
			if ep == nil {
				return nil
			}

			if blockNumber >= ep.StartBlock && blockNumber <= ep.EndBlock {
				return ep
			}
		}
	}
	return nil
}

func saveId(db dbm.DB, chainId string) {

	buf := db.Get(allChainKey)

	if len(buf) == 0 {
		db.SetSync(allChainKey, []byte(chainId))
		log.Debugf("ChainInfo SaveId(), chainId is: %s", chainId)
	} else {

		strIdArr := strings.Split(string(buf), specialSep)

		found := false
		for _, id := range strIdArr {
			if id == chainId {
				found = true
				break
			}
		}

		if !found {
			strIdArr = append(strIdArr, chainId)
			strIds := strings.Join(strIdArr, specialSep)
			db.SetSync(allChainKey, []byte(strIds))

			log.Debugf("ChainInfo SaveId(), strIds is: %s", strIds)
		}
	}
}

func GetSideChainIds(db dbm.DB) []string {
	mtx.RLock()
	defer mtx.RUnlock()

	buf := db.Get(allChainKey)

	log.Debugf("Get side chain IDs, buf is %v, len is %d", buf, len(buf))

	if len(buf) == 0 {
		return []string{}
	}

	return strings.Split(string(buf), specialSep)
}

func CheckSideChainRunning(db dbm.DB, chainId string) bool {
	ids := GetSideChainIds(db)

	for _, id := range ids {
		if id == chainId {
			return true
		}
	}

	return false
}

func SaveChainGenesis(db dbm.DB, chainId string, ethGenesis, ntcGenesis []byte) {
	mtx.Lock()
	defer mtx.Unlock()

	db.SetSync(calcETHGenesisKey(chainId), ethGenesis)

	db.SetSync(calcNTCGenesisKey(chainId), ntcGenesis)
}

func LoadChainGenesis(db dbm.DB, chainId string) (ethGenesis, ntcGenesis []byte) {
	mtx.RLock()
	defer mtx.RUnlock()

	ethGenesis = db.Get(calcETHGenesisKey(chainId))
	ntcGenesis = db.Get(calcNTCGenesisKey(chainId))
	return
}

var pendingChainMtx sync.Mutex

var pendingChainIndexKey = []byte("PENDING_CHAIN_IDX")

func calcPendingChainInfoKey(chainId string) []byte {
	return []byte("PENDING_CHAIN:" + chainId)
}

type pendingIdxData struct {
	ChainID string
	Start   *big.Int
	End     *big.Int
}

func GetPendingSideChainData(db dbm.DB, chainId string) *CoreChainInfo {

	pendingChainByteSlice := db.Get(calcPendingChainInfoKey(chainId))
	if pendingChainByteSlice != nil {
		var cci CoreChainInfo
		wire.ReadBinaryBytes(pendingChainByteSlice, &cci)
		return &cci
	}

	return nil
}

func CreatePendingSideChainData(db dbm.DB, cci *CoreChainInfo) {
	storePendingSideChainData(db, cci, true)
}

func UpdatePendingSideChainData(db dbm.DB, cci *CoreChainInfo) {
	storePendingSideChainData(db, cci, false)
}

func storePendingSideChainData(db dbm.DB, cci *CoreChainInfo, create bool) {
	pendingChainMtx.Lock()
	defer pendingChainMtx.Unlock()

	db.SetSync(calcPendingChainInfoKey(cci.ChainId), wire.BinaryBytes(*cci))

	if create {

		var idx []pendingIdxData
		pendingIdxByteSlice := db.Get(pendingChainIndexKey)
		if pendingIdxByteSlice != nil {
			wire.ReadBinaryBytes(pendingIdxByteSlice, &idx)
		}

		for _, v := range idx {
			if v.ChainID == cci.ChainId {
				return
			}
		}

		idx = append(idx, pendingIdxData{cci.ChainId, cci.StartBlock, cci.EndBlock})
		db.SetSync(pendingChainIndexKey, wire.BinaryBytes(idx))
	}
}

func DeletePendingSideChainData(db dbm.DB, chainId string) {
	pendingChainMtx.Lock()
	defer pendingChainMtx.Unlock()

	db.DeleteSync(calcPendingChainInfoKey(chainId))
}

func GetSideChainForLaunch(db dbm.DB, height *big.Int, stateDB *state.StateDB) (readyForLaunch []string, newPendingIdxBytes []byte, deleteSideChainIds []string) {
	pendingChainMtx.Lock()
	defer pendingChainMtx.Unlock()

	var idx []pendingIdxData
	pendingIdxByteSlice := db.Get(pendingChainIndexKey)
	if pendingIdxByteSlice != nil {
		wire.ReadBinaryBytes(pendingIdxByteSlice, &idx)
	}

	if len(idx) == 0 {
		return
	}

	newPendingIdx := idx[:0]

	for _, v := range idx {
		if v.Start.Cmp(height) > 0 {

			newPendingIdx = append(newPendingIdx, v)
		} else if v.End.Cmp(height) < 0 {

			cci := GetPendingSideChainData(db, v.ChainID)
			for _, jv := range cci.JoinedValidators {
				stateDB.SubSideChainDepositBalance(jv.Address, v.ChainID, jv.DepositAmount)
				stateDB.AddBalance(jv.Address, jv.DepositAmount)
			}

			officialMinimumDeposit := math.MustParseBig256(OFFICIAL_MINIMUM_DEPOSIT)
			stateDB.AddBalance(cci.Owner, officialMinimumDeposit)
			stateDB.SubChainBalance(cci.Owner, officialMinimumDeposit)
			if stateDB.GetChainBalance(cci.Owner).Sign() != 0 {
				log.Error("the chain balance is not 0 when create chain failed, watch out!!!")
			}

			deleteSideChainIds = append(deleteSideChainIds, v.ChainID)

		} else {

			cci := GetPendingSideChainData(db, v.ChainID)
			if len(cci.JoinedValidators) >= int(cci.MinValidators) && cci.TotalDeposit().Cmp(cci.MinDepositAmount) >= 0 {

				for _, jv := range cci.JoinedValidators {

					stateDB.SubSideChainDepositBalance(jv.Address, v.ChainID, jv.DepositAmount)
					stateDB.AddChainBalance(cci.Owner, jv.DepositAmount)
				}

				readyForLaunch = append(readyForLaunch, v.ChainID)
			} else {
				newPendingIdx = append(newPendingIdx, v)
			}
		}
	}

	if len(newPendingIdx) != len(idx) {

		newPendingIdxBytes = wire.BinaryBytes(newPendingIdx)

	}

	return
}

func ProcessPostPendingData(db dbm.DB, newPendingIdxBytes []byte, deleteSideChainIds []string) {
	pendingChainMtx.Lock()
	defer pendingChainMtx.Unlock()

	for _, id := range deleteSideChainIds {
		db.DeleteSync(calcPendingChainInfoKey(id))
	}

	if newPendingIdxBytes != nil {
		db.SetSync(pendingChainIndexKey, newPendingIdxBytes)
	}
}
