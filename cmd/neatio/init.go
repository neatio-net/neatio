package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neatlab/neatio/common/hexutil"
	"github.com/neatlab/neatio/common/math"
	"github.com/neatlab/neatio/core/rawdb"
	"github.com/neatlab/neatio/log"
	"github.com/neatlab/neatio/neatabi/abi"

	"gopkg.in/urfave/cli.v1"

	"encoding/json"
	"io/ioutil"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/neatlab/neatio/accounts/keystore"
	"github.com/neatlab/neatio/cmd/utils"
	"github.com/neatlab/neatio/common"
	"github.com/neatlab/neatio/consensus/neatpos/types"
	"github.com/neatlab/neatio/core"
	"github.com/neatlab/neatio/params"
	cmn "github.com/neatlib/common-go"
	cfg "github.com/neatlib/config-go"
	dbm "github.com/neatlib/db-go"
	"github.com/pkg/errors"
)

const (
	POSReward = "5000000000000000000000000000" // 50 Billions

	TotalYear = 30

	DefaultAccountPassword = "neatio"
)

type BalaceAmount struct {
	balance string
	amount  string
}

type InvalidArgs struct {
	args string
}

func (invalid InvalidArgs) Error() string {
	return "invalid args:" + invalid.args
}

func initNeatGenesis(ctx *cli.Context) error {
	log.Info("this is Neatio genesis initialization")
	args := ctx.Args()
	if len(args) != 1 {
		utils.Fatalf("len of args is %d", len(args))
		return nil
	}
	balance_str := args[0]

	chainId := MainChain
	isMainnet := true
	if ctx.GlobalBool(utils.TestnetFlag.Name) {
		chainId = TestnetChain
		isMainnet = false
	}
	log.Infof("this is init-neatio chainId %v", chainId)
	log.Info("this is init-neatio" + ctx.GlobalString(utils.DataDirFlag.Name) + "--" + ctx.Args()[0])
	return init_neat_genesis(utils.GetNeatConConfig(chainId, ctx), balance_str, isMainnet)
}

func init_neat_genesis(config cfg.Config, balanceStr string, isMainnet bool) error {

	balanceAmounts, err := parseBalaceAmount(balanceStr)
	if err != nil {
		utils.Fatalf("init neatio failed")
		return err
	}

	validators := createPriValidators(config, len(balanceAmounts))
	extraData, _ := hexutil.Decode("0x0")

	var chainConfig *params.ChainConfig
	if isMainnet {
		chainConfig = params.MainnetChainConfig
	} else {
		chainConfig = params.TestnetChainConfig
	}

	var coreGenesis = core.GenesisWrite{
		Config:     chainConfig,
		Nonce:      0xdeadbeefdeadbeef,
		Timestamp:  uint64(time.Now().Unix()),
		ParentHash: common.Hash{},
		ExtraData:  extraData,
		GasLimit:   0x7270e00,
		Difficulty: new(big.Int).SetUint64(0x01),
		Mixhash:    common.Hash{},
		Coinbase:   "NEATAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Alloc:      core.GenesisAllocWrite{},
	}
	for i, validator := range validators {
		coreGenesis.Alloc[validator.Address.String()] = core.GenesisAccount{
			Balance: math.MustParseBig256(balanceAmounts[i].balance),
			Amount:  math.MustParseBig256(balanceAmounts[i].amount),
		}
	}

	contents, err := json.MarshalIndent(coreGenesis, "", "\t")
	if err != nil {
		utils.Fatalf("marshal coreGenesis failed")
		return err
	}
	neatGenesisPath := config.GetString("neat_genesis_file")

	if err = ioutil.WriteFile(neatGenesisPath, contents, 0654); err != nil {
		utils.Fatalf("write neat_genesis_file failed")
		return err
	}
	return nil
}

func initCmd(ctx *cli.Context) error {

	// Neatio genesis.json
	neatGenesisPath := ctx.Args().First()
	fmt.Printf("Neatio genesis path %v\n", neatGenesisPath)
	if len(neatGenesisPath) == 0 {
		utils.Fatalf("must supply path to genesis JSON file")
	}

	chainId := ctx.Args().Get(1)
	if chainId == "" {
		chainId = MainChain
		if ctx.GlobalBool(utils.TestnetFlag.Name) {
			chainId = TestnetChain
		}
	}

	return init_cmd(ctx, utils.GetNeatConConfig(chainId, ctx), chainId, neatGenesisPath)
}

func InitSideChainCmd(ctx *cli.Context) error {
	// Load ChainInfo db
	chainInfoDb := dbm.NewDB("chaininfo", "leveldb", ctx.GlobalString(utils.DataDirFlag.Name))
	if chainInfoDb == nil {
		return errors.New("could not open chain info database")
	}
	defer chainInfoDb.Close()

	// Initial Child Chain Genesis
	sideChainIds := ctx.GlobalString("sideChain")
	if sideChainIds == "" {
		return errors.New("please provide side chain id to initialization")
	}

	chainIds := strings.Split(sideChainIds, ",")
	for _, chainId := range chainIds {
		ethGenesis, tdmGenesis := core.LoadChainGenesis(chainInfoDb, chainId)
		if ethGenesis == nil || tdmGenesis == nil {
			return errors.New(fmt.Sprintf("unable to retrieve the genesis file for side chain %s", chainId))
		}

		childConfig := utils.GetNeatConConfig(chainId, ctx)

		// Write down genesis and get the genesis path
		ethGenesisPath := childConfig.GetString("neat_genesis_file")
		if err := ioutil.WriteFile(ethGenesisPath, ethGenesis, 0644); err != nil {
			utils.Fatalf("write neat_genesis_file failed")
			return err
		}

		// Init the blockchain from genesis path
		init_neat_blockchain(chainId, ethGenesisPath, ctx)

		// Write down TDM Genesis directly
		if err := ioutil.WriteFile(childConfig.GetString("genesis_file"), tdmGenesis, 0644); err != nil {
			utils.Fatalf("write tdm genesis_file failed")
			return err
		}

	}

	return nil
}

func init_cmd(ctx *cli.Context, config cfg.Config, chainId string, neatGenesisPath string) error {

	init_neat_blockchain(chainId, neatGenesisPath, ctx)

	init_em_files(config, chainId, neatGenesisPath, nil)

	return nil
}

func init_neat_blockchain(chainId string, neatGenesisPath string, ctx *cli.Context) {

	dbPath := filepath.Join(utils.MakeDataDir(ctx), chainId, clientIdentifier, "/chaindata")
	log.Infof("init_neat_blockchain 0 with dbPath: %s", dbPath)

	chainDb, err := rawdb.NewLevelDBDatabase(filepath.Join(utils.MakeDataDir(ctx), chainId, clientIdentifier, "/chaindata"), 0, 0, "neatio/db/chaindata/")
	if err != nil {
		utils.Fatalf("could not open database: %v", err)
	}
	defer chainDb.Close()

	log.Info("init_neat_blockchain 1")
	genesisFile, err := os.Open(neatGenesisPath)
	if err != nil {
		utils.Fatalf("failed to read genesis file: %v", err)
	}
	defer genesisFile.Close()

	log.Info("init_neat_blockchain 2")
	block, err := core.WriteGenesisBlock(chainDb, genesisFile)
	if err != nil {
		utils.Fatalf("failed to write genesis block: %v", err)
	}

	log.Info("init_neat_blockchain end")
	log.Infof("successfully wrote genesis block and/or chain rule set: %x", block.Hash())
}

func init_em_files(config cfg.Config, chainId string, genesisPath string, validators []types.GenesisValidator) error {
	gensisFile, err := os.Open(genesisPath)
	defer gensisFile.Close()
	if err != nil {
		utils.Fatalf("failed to read neatio genesis file: %v", err)
		return err
	}
	contents, err := ioutil.ReadAll(gensisFile)
	if err != nil {
		utils.Fatalf("failed to read neatio genesis file: %v", err)
		return err
	}
	var (
		genesisW    core.GenesisWrite
		coreGenesis core.Genesis
	)
	if err := json.Unmarshal(contents, &genesisW); err != nil {
		return err
	}

	coreGenesis = core.Genesis{
		Config:     genesisW.Config,
		Nonce:      genesisW.Nonce,
		Timestamp:  genesisW.Timestamp,
		ParentHash: genesisW.ParentHash,
		ExtraData:  genesisW.ExtraData,
		GasLimit:   genesisW.GasLimit,
		Difficulty: genesisW.Difficulty,
		Mixhash:    genesisW.Mixhash,
		Coinbase:   common.StringToAddress(genesisW.Coinbase),
		Alloc:      core.GenesisAlloc{},
	}

	for k, v := range genesisW.Alloc {
		coreGenesis.Alloc[common.StringToAddress(k)] = v
	}

	var privValidator *types.PrivValidator
	// validators == nil means we are init the Genesis from priv_validator, not from runtime GenesisValidator
	if validators == nil {
		privValPath := config.GetString("priv_validator_file")
		if _, err := os.Stat(privValPath); os.IsNotExist(err) {
			log.Info("priv_validator_file not exist, probably you are running in non-mining mode")
			return nil
		}
		// Now load the priv_validator_file
		privValidator = types.LoadPrivValidator(privValPath)
	}

	// Create the Genesis Doc
	if err := createGenesisDoc(config, chainId, &coreGenesis, privValidator, validators); err != nil {
		utils.Fatalf("failed to write genesis file: %v", err)
		return err
	}
	return nil
}

func createGenesisDoc(config cfg.Config, chainId string, coreGenesis *core.Genesis, privValidator *types.PrivValidator, validators []types.GenesisValidator) error {
	genFile := config.GetString("genesis_file")
	if _, err := os.Stat(genFile); os.IsNotExist(err) {

		posReward, _ := new(big.Int).SetString(POSReward, 10)
		totalYear := TotalYear
		rewardFirstYear := new(big.Int).Div(posReward, big.NewInt(int64(totalYear)))

		var rewardScheme types.RewardSchemeDoc
		if chainId == MainChain || chainId == TestnetChain {
			rewardScheme = types.RewardSchemeDoc{
				TotalReward:        posReward,
				RewardFirstYear:    rewardFirstYear,
				EpochNumberPerYear: 2191,
				TotalYear:          uint64(totalYear),
			}
		} else {
			rewardScheme = types.RewardSchemeDoc{
				TotalReward:        big.NewInt(0),
				RewardFirstYear:    big.NewInt(0),
				EpochNumberPerYear: 1,
				TotalYear:          0,
			}
		}

		var rewardPerBlock *big.Int
		if chainId == MainChain || chainId == TestnetChain {
			rewardPerBlock = new(big.Int).Mul(big.NewInt(1e+18), big.NewInt(50)) // 50 NEAT per block
		} else {
			rewardPerBlock = big.NewInt(0)
		}

		fmt.Printf("Init reward block %v\n", rewardPerBlock)
		genDoc := types.GenesisDoc{
			ChainID:      chainId,
			Consensus:    types.Consensus_NeatPoS,
			GenesisTime:  time.Now(),
			RewardScheme: rewardScheme,
			CurrentEpoch: types.OneEpochDoc{
				Number:         0,
				RewardPerBlock: rewardPerBlock,
				StartBlock:     0,
				EndBlock:       14400, // 4h epoch
				Status:         0,
			},
		}

		if privValidator != nil {
			coinbase, amount, checkErr := checkAccount(*coreGenesis)
			if checkErr != nil {
				log.Infof(checkErr.Error())
				cmn.Exit(checkErr.Error())
			}

			genDoc.CurrentEpoch.Validators = []types.GenesisValidator{{
				EthAccount: coinbase,
				PubKey:     privValidator.PubKey,
				Amount:     amount,
			}}
		} else if validators != nil {
			genDoc.CurrentEpoch.Validators = validators
		}
		genDoc.SaveAs(genFile)
	}
	return nil
}

func generateNCGenesis(sideChainID string, validators []types.GenesisValidator) ([]byte, error) {
	var rewardScheme = types.RewardSchemeDoc{
		TotalReward:        big.NewInt(0),
		RewardFirstYear:    big.NewInt(0),
		EpochNumberPerYear: 12,
		TotalYear:          0,
	}

	genDoc := types.GenesisDoc{
		ChainID:      sideChainID,
		Consensus:    types.Consensus_NeatPoS,
		GenesisTime:  time.Now(),
		RewardScheme: rewardScheme,
		CurrentEpoch: types.OneEpochDoc{
			Number:         0,
			RewardPerBlock: big.NewInt(0),
			StartBlock:     0,
			EndBlock:       657000,
			Status:         0,
			Validators:     validators,
		},
	}

	contents, err := json.Marshal(genDoc)
	if err != nil {
		utils.Fatalf("Marshal NC Genesis failed")
		return nil, err
	}
	return contents, nil
}

func parseBalaceAmount(s string) ([]*BalaceAmount, error) {
	r, _ := regexp.Compile("\\{[\\ \\t]*\\d+(\\.\\d+)?[\\ \\t]*\\,[\\ \\t]*\\d+(\\.\\d+)?[\\ \\t]*\\}")
	parse_strs := r.FindAllString(s, -1)
	if len(parse_strs) == 0 {
		return nil, InvalidArgs{s}
	}
	balanceAmounts := make([]*BalaceAmount, len(parse_strs))
	for i, v := range parse_strs {
		length := len(v)
		balanceAmount := strings.Split(v[1:length-1], ",")
		if len(balanceAmount) != 2 {
			return nil, InvalidArgs{s}
		}
		balanceAmounts[i] = &BalaceAmount{strings.TrimSpace(balanceAmount[0]), strings.TrimSpace(balanceAmount[1])}
	}
	return balanceAmounts, nil
}

func createPriValidators(config cfg.Config, num int) []*types.PrivValidator {
	validators := make([]*types.PrivValidator, num)

	ks := keystore.NewKeyStore(config.GetString("keystore"), keystore.StandardScryptN, keystore.StandardScryptP)

	privValFile := config.GetString("priv_validator_file_root")
	for i := 0; i < num; i++ {
		// Create New NeatBlockchain Account
		account, err := ks.NewAccount(DefaultAccountPassword)
		if err != nil {
			utils.Fatalf("Failed to create NeatBlockchain account: %v", err)
		}
		// Generate Consensus KeyPair
		validators[i] = types.GenPrivValidatorKey(account.Address)
		log.Info("Creating Priv Validators File", "account:", validators[i].Address, "pwd:", DefaultAccountPassword)
		if i > 0 {
			validators[i].SetFile(privValFile + strconv.Itoa(i) + ".json")
		} else {
			validators[i].SetFile(privValFile + ".json")
		}
		validators[i].Save()
	}
	return validators
}

func checkAccount(coreGenesis core.Genesis) (common.Address, *big.Int, error) {

	coinbase := coreGenesis.Coinbase
	log.Infof("checkAccount(), coinbase is %v", coinbase.String())

	var act common.Address
	amount := big.NewInt(-1)
	balance := big.NewInt(-1)
	found := false
	for address, account := range coreGenesis.Alloc {
		log.Infof("checkAccount(), address is %v, balance is %v, amount is %v", address.String(), account.Balance, account.Amount)
		balance = account.Balance
		amount = account.Amount
		act = address
		found = true
		break
	}

	if !found {
		log.Error("invalidate eth_account")
		return common.Address{}, nil, errors.New("invalidate eth_account")
	}

	if balance.Sign() == -1 || amount.Sign() == -1 {
		log.Errorf("balance / amount can't be negative integer, balance is %v, amount is %v", balance, amount)
		return common.Address{}, nil, errors.New("no enough balance")
	}

	return act, amount, nil
}

func initEthGenesisFromExistValidator(sideChainID string, childConfig cfg.Config, validators []types.GenesisValidator) error {

	contents, err := generateETHGenesis(sideChainID, validators)
	if err != nil {
		return err
	}
	ethGenesisPath := childConfig.GetString("neat_genesis_file")
	if err = ioutil.WriteFile(ethGenesisPath, contents, 0654); err != nil {
		utils.Fatalf("write neat_genesis_file failed")
		return err
	}
	return nil
}

func generateETHGenesis(sideChainID string, validators []types.GenesisValidator) ([]byte, error) {
	var coreGenesis = core.Genesis{
		Config:     params.NewSideChainConfig(sideChainID),
		Nonce:      0xdeadbeefdeadbeef,
		Timestamp:  0x0,
		ParentHash: common.Hash{},
		ExtraData:  []byte("0x0"),
		GasLimit:   0x8000000,
		Difficulty: new(big.Int).SetUint64(0x400),
		Mixhash:    common.Hash{},
		Coinbase:   common.Address{},
		Alloc:      core.GenesisAlloc{},
	}
	for _, validator := range validators {
		coreGenesis.Alloc[validator.EthAccount] = core.GenesisAccount{
			Balance: big.NewInt(0),
			Amount:  validator.Amount,
		}
	}

	// Add Child Chain Default Token
	coreGenesis.Alloc[abi.SideChainTokenIncentiveAddr] = core.GenesisAccount{
		Balance: new(big.Int).Mul(big.NewInt(100000), big.NewInt(1e+18)),
		Amount:  common.Big0,
	}

	contents, err := json.Marshal(coreGenesis)
	if err != nil {
		utils.Fatalf("marshal coreGenesis failed")
		return nil, err
	}
	return contents, nil
}
