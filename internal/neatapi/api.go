package neatapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/neatlab/neatio/chain/consensus"
	"github.com/neatlab/neatio/chain/consensus/neatcon/epoch"
	"github.com/neatlab/neatio/chain/core/state"

	"github.com/neatio-network/neatio/chain/accounts"
	"github.com/neatio-network/neatio/chain/accounts/abi"
	"github.com/neatio-network/neatio/chain/accounts/keystore"
	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/chain/core/rawdb"
	"github.com/neatio-network/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/core/vm"
	"github.com/neatio-network/neatio/chain/log"
	neatAbi "github.com/neatlab/neatio/neatabi/abi"
	"github.com/neatlab/neatio/network/p2p"
	"github.com/neatlab/neatio/network/rpc"
	"github.com/neatlab/neatio/params"
	"github.com/neatio-network/neatio/utilities/common"
	"github.com/neatio-network/neatio/utilities/common/hexutil"
	"github.com/neatio-network/neatio/utilities/common/math"
	"github.com/neatio-network/neatio/utilities/crypto"
	"github.com/neatlab/neatio/utilities/rlp"
	goCrypto "github.com/neatlib/crypto-go"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	defaultGasPrice          = 500 * params.GWei
	updateValidatorThreshold = 50
)

var (
	minimumRegisterAmount = math.MustParseBig256("50000000000000000000000")

	maxDelegationAddresses = 1000

	maxEditValidatorLength = 100
)

type PublicNEATChainAPI struct {
	b Backend
}

func NewPublicNEATChainAPI(b Backend) *PublicNEATChainAPI {
	return &PublicNEATChainAPI{b}
}

func (s *PublicNEATChainAPI) GasPrice(ctx context.Context) (*hexutil.Big, error) {
	price, err := s.b.SuggestPrice(ctx)
	return (*hexutil.Big)(price), err
}

func (s *PublicNEATChainAPI) ProtocolVersion() hexutil.Uint {
	return hexutil.Uint(s.b.ProtocolVersion())
}

func (s *PublicNEATChainAPI) Syncing() (interface{}, error) {
	progress := s.b.Downloader().Progress()

	if progress.CurrentBlock >= progress.HighestBlock {
		return false, nil
	}

	return map[string]interface{}{
		"startingBlock": hexutil.Uint64(progress.StartingBlock),
		"currentBlock":  hexutil.Uint64(progress.CurrentBlock),
		"highestBlock":  hexutil.Uint64(progress.HighestBlock),
		"pulledStates":  hexutil.Uint64(progress.PulledStates),
		"knownStates":   hexutil.Uint64(progress.KnownStates),
	}, nil
}

type PublicTxPoolAPI struct {
	b Backend
}

func NewPublicTxPoolAPI(b Backend) *PublicTxPoolAPI {
	return &PublicTxPoolAPI{b}
}

func (s *PublicTxPoolAPI) Content() map[string]map[string]map[string]*RPCTransaction {
	content := map[string]map[string]map[string]*RPCTransaction{
		"pending": make(map[string]map[string]*RPCTransaction),
		"queued":  make(map[string]map[string]*RPCTransaction),
	}
	pending, queue := s.b.TxPoolContent()

	for account, txs := range pending {
		dump := make(map[string]*RPCTransaction)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = newRPCPendingTransaction(tx)
		}
		content["pending"][account.Hex()] = dump
	}

	for account, txs := range queue {
		dump := make(map[string]*RPCTransaction)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = newRPCPendingTransaction(tx)
		}
		content["queued"][account.Hex()] = dump
	}
	return content
}

func (s *PublicTxPoolAPI) Status() map[string]hexutil.Uint {
	pending, queue := s.b.Stats()
	return map[string]hexutil.Uint{
		"pending": hexutil.Uint(pending),
		"queued":  hexutil.Uint(queue),
	}
}

func (s *PublicTxPoolAPI) Inspect() map[string]map[string]map[string]string {
	content := map[string]map[string]map[string]string{
		"pending": make(map[string]map[string]string),
		"queued":  make(map[string]map[string]string),
	}
	pending, queue := s.b.TxPoolContent()

	var format = func(tx *types.Transaction) string {
		if to := tx.To(); to != nil {
			return fmt.Sprintf("%s: %v wei + %v gas × %v wei", tx.To().Hex(), tx.Value(), tx.Gas(), tx.GasPrice())
		}
		return fmt.Sprintf("contract creation: %v wei + %v gas × %v wei", tx.Value(), tx.Gas(), tx.GasPrice())
	}

	for account, txs := range pending {
		dump := make(map[string]string)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = format(tx)
		}
		content["pending"][account.Hex()] = dump
	}

	for account, txs := range queue {
		dump := make(map[string]string)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = format(tx)
		}
		content["queued"][account.Hex()] = dump
	}
	return content
}

type PublicAccountAPI struct {
	am *accounts.Manager
}

func NewPublicAccountAPI(am *accounts.Manager) *PublicAccountAPI {
	return &PublicAccountAPI{am: am}
}

func (s *PublicAccountAPI) Accounts() []common.Address {
	addresses := make([]common.Address, 0)
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address)
		}
	}
	return addresses
}

type PrivateAccountAPI struct {
	am        *accounts.Manager
	nonceLock *AddrLocker
	b         Backend
}

func NewPrivateAccountAPI(b Backend, nonceLock *AddrLocker) *PrivateAccountAPI {
	return &PrivateAccountAPI{
		am:        b.AccountManager(),
		nonceLock: nonceLock,
		b:         b,
	}
}

func (s *PrivateAccountAPI) ListAccounts() []common.Address {
	addresses := make([]common.Address, 0)
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address)
		}
	}
	return addresses
}

type rawWallet struct {
	URL      string             `json:"url"`
	Status   string             `json:"status"`
	Failure  string             `json:"failure,omitempty"`
	Accounts []accounts.Account `json:"accounts,omitempty"`
}

func (s *PrivateAccountAPI) ListWallets() []rawWallet {
	wallets := make([]rawWallet, 0)
	for _, wallet := range s.am.Wallets() {
		status, failure := wallet.Status()

		raw := rawWallet{
			URL:      wallet.URL().String(),
			Status:   status,
			Accounts: wallet.Accounts(),
		}
		if failure != nil {
			raw.Failure = failure.Error()
		}
		wallets = append(wallets, raw)
	}
	return wallets
}

func (s *PrivateAccountAPI) OpenWallet(url string, passphrase *string) error {
	wallet, err := s.am.Wallet(url)
	if err != nil {
		return err
	}
	pass := ""
	if passphrase != nil {
		pass = *passphrase
	}
	return wallet.Open(pass)
}

func (s *PrivateAccountAPI) DeriveAccount(url string, path string, pin *bool) (accounts.Account, error) {
	wallet, err := s.am.Wallet(url)
	if err != nil {
		return accounts.Account{}, err
	}
	derivPath, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return accounts.Account{}, err
	}
	if pin == nil {
		pin = new(bool)
	}
	return wallet.Derive(derivPath, *pin)
}

func (s *PrivateAccountAPI) NewAccount(password string) (common.Address, error) {
	acc, err := fetchKeystore(s.am).NewAccount(password)
	if err == nil {
		return acc.Address, nil
	}
	return common.Address{}, err
}

func fetchKeystore(am *accounts.Manager) *keystore.KeyStore {
	return am.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
}

func (s *PrivateAccountAPI) ImportRawKey(privkey string, password string) (common.Address, error) {
	key, err := crypto.HexToECDSA(privkey)
	if err != nil {
		return common.Address{}, err
	}
	acc, err := fetchKeystore(s.am).ImportECDSA(key, password)
	return acc.Address, err
}

func (s *PrivateAccountAPI) UnlockAccount(addr common.Address, password string, duration *uint64) (bool, error) {
	const max = uint64(time.Duration(math.MaxInt64) / time.Second)
	var d time.Duration
	if duration == nil {
		d = 300 * time.Second
	} else if *duration > max {
		return false, errors.New("unlock duration too large")
	} else {
		d = time.Duration(*duration) * time.Second
	}
	err := fetchKeystore(s.am).TimedUnlock(accounts.Account{Address: addr}, password, d)
	return err == nil, err
}

func (s *PrivateAccountAPI) LockAccount(addr common.Address) bool {
	return fetchKeystore(s.am).Lock(addr) == nil
}

func (s *PrivateAccountAPI) signTransaction(ctx context.Context, args SendTxArgs, passwd string) (*types.Transaction, error) {

	account := accounts.Account{Address: args.From}
	wallet, err := s.am.Find(account)
	if err != nil {
		return nil, err
	}

	if err := args.setDefaults(ctx, s.b); err != nil {
		return nil, err
	}

	tx := args.toTransaction()

	var chainID *big.Int
	if config := s.b.ChainConfig(); config.IsEIP155(s.b.CurrentBlock().Number()) {
		chainID = config.ChainId
	}
	return wallet.SignTxWithPassphrase(account, passwd, tx, chainID)
}

func (s *PrivateAccountAPI) SendTransaction(ctx context.Context, args SendTxArgs, passwd string) (common.Hash, error) {
	fmt.Printf("transaction args PrivateAccountAPI args %v\n", args)
	if args.Nonce == nil {

		s.nonceLock.LockAddr(args.From)
		defer s.nonceLock.UnlockAddr(args.From)
	}
	signed, err := s.signTransaction(ctx, args, passwd)
	if err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, s.b, signed)
}

func (s *PrivateAccountAPI) SignTransaction(ctx context.Context, args SendTxArgs, passwd string) (*SignTransactionResult, error) {

	if args.Gas == nil {
		return nil, fmt.Errorf("gas not specified")
	}
	if args.GasPrice == nil {
		return nil, fmt.Errorf("gasPrice not specified")
	}
	if args.Nonce == nil {
		return nil, fmt.Errorf("nonce not specified")
	}
	signed, err := s.signTransaction(ctx, args, passwd)
	if err != nil {
		return nil, err
	}
	data, err := rlp.EncodeToBytes(signed)
	if err != nil {
		return nil, err
	}
	return &SignTransactionResult{data, signed}, nil
}

func signHash(data []byte) []byte {
	msg := fmt.Sprintf("\x19NEAT Chain Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256([]byte(msg))
}

func (s *PrivateAccountAPI) Sign(ctx context.Context, data hexutil.Bytes, addr common.Address, passwd string) (hexutil.Bytes, error) {

	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}

	signature, err := wallet.SignHashWithPassphrase(account, passwd, signHash(data))
	if err != nil {
		return nil, err
	}
	signature[64] += 27
	return signature, nil
}

func (s *PrivateAccountAPI) EcRecover(ctx context.Context, data, sig hexutil.Bytes) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, fmt.Errorf("signature must be 65 bytes long")
	}
	if sig[64] != 27 && sig[64] != 28 {
		return common.Address{}, fmt.Errorf("invalid Neatio signature (V is not 27 or 28)")
	}
	sig[64] -= 27

	rpk, err := crypto.SigToPub(signHash(data), sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*rpk), nil
}

func (s *PrivateAccountAPI) SignAndSendTransaction(ctx context.Context, args SendTxArgs, passwd string) (common.Hash, error) {
	return s.SendTransaction(ctx, args, passwd)
}

type PublicBlockChainAPI struct {
	b Backend
}

func NewPublicBlockChainAPI(b Backend) *PublicBlockChainAPI {
	return &PublicBlockChainAPI{b}
}

func (s *PublicBlockChainAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(s.b.ChainConfig().ChainId)
}

func (s *PublicBlockChainAPI) BlockNumber() hexutil.Uint64 {
	header, _ := s.b.HeaderByNumber(context.Background(), rpc.LatestBlockNumber)
	return hexutil.Uint64(header.Number.Uint64())
}

func (s *PublicBlockChainAPI) GetBalance(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (*hexutil.Big, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	return (*hexutil.Big)(state.GetBalance(address)), state.Error()
}

type ProxiedDetail struct {
	ProxiedBalance        *hexutil.Big `json:"proxiedBalance"`
	DepositProxiedBalance *hexutil.Big `json:"depositProxiedBalance"`
	PendingRefundBalance  *hexutil.Big `json:"pendingRefundBalance"`
}

func (s *PublicBlockChainAPI) GetBalanceDetail(ctx context.Context, address common.Address, blockNr rpc.BlockNumber, fullDetail bool) (map[string]interface{}, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}

	fields := map[string]interface{}{
		"balance":               (*hexutil.Big)(state.GetBalance(address)),
		"depositBalance":        (*hexutil.Big)(state.GetDepositBalance(address)),
		"delegateBalance":       (*hexutil.Big)(state.GetDelegateBalance(address)),
		"proxiedBalance":        (*hexutil.Big)(state.GetTotalProxiedBalance(address)),
		"depositProxiedBalance": (*hexutil.Big)(state.GetTotalDepositProxiedBalance(address)),
		"pendingRefundBalance":  (*hexutil.Big)(state.GetTotalPendingRefundBalance(address)),
		"rewardBalance":         (*hexutil.Big)(state.GetTotalRewardBalance(address)),
	}

	if fullDetail {
		proxiedDetail := make(map[common.Address]ProxiedDetail)
		state.ForEachProxied(address, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
			proxiedDetail[key] = ProxiedDetail{
				ProxiedBalance:        (*hexutil.Big)(proxiedBalance),
				DepositProxiedBalance: (*hexutil.Big)(depositProxiedBalance),
				PendingRefundBalance:  (*hexutil.Big)(pendingRefundBalance),
			}
			return true
		})

		fields["proxiedDetail"] = proxiedDetail

		rewardDetail := make(map[common.Address]*hexutil.Big)
		state.ForEachReward(address, func(key common.Address, rewardBalance *big.Int) bool {
			rewardDetail[key] = (*hexutil.Big)(rewardBalance)
			return true
		})

		fields["rewardDetail"] = rewardDetail
	}
	return fields, state.Error()
}

type EpochLabel uint64

func (e EpochLabel) MarshalText() ([]byte, error) {
	output := fmt.Sprintf("epoch_%d", e)
	return []byte(output), nil
}

func (s *PublicBlockChainAPI) GetBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber, fullTx bool) (map[string]interface{}, error) {
	block, err := s.b.BlockByNumber(ctx, blockNr)
	if block != nil {
		response, err := s.rpcOutputBlock(block, true, fullTx)
		if err == nil && blockNr == rpc.PendingBlockNumber {

			for _, field := range []string{"hash", "nonce", "miner"} {
				response[field] = nil
			}
		}
		return response, err
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetBlockByHash(ctx context.Context, blockHash common.Hash, fullTx bool) (map[string]interface{}, error) {
	block, err := s.b.GetBlock(ctx, blockHash)
	if block != nil {
		return s.rpcOutputBlock(block, true, fullTx)
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetUncleByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint) (map[string]interface{}, error) {
	block, err := s.b.BlockByNumber(ctx, blockNr)
	if block != nil {
		uncles := block.Uncles()
		if index >= hexutil.Uint(len(uncles)) {
			log.Debug("Requested uncle not found", "number", blockNr, "hash", block.Hash(), "index", index)
			return nil, nil
		}
		block = types.NewBlockWithHeader(uncles[index])
		return s.rpcOutputBlock(block, false, false)
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetUncleByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) (map[string]interface{}, error) {
	block, err := s.b.GetBlock(ctx, blockHash)
	if block != nil {
		uncles := block.Uncles()
		if index >= hexutil.Uint(len(uncles)) {
			log.Debug("Requested uncle not found", "number", block.Number(), "hash", blockHash, "index", index)
			return nil, nil
		}
		block = types.NewBlockWithHeader(uncles[index])
		return s.rpcOutputBlock(block, false, false)
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetUncleCountByBlockNumber(ctx context.Context, blockNr rpc.BlockNumber) *hexutil.Uint {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		n := hexutil.Uint(len(block.Uncles()))
		return &n
	}
	return nil
}

func (s *PublicBlockChainAPI) GetUncleCountByBlockHash(ctx context.Context, blockHash common.Hash) *hexutil.Uint {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		n := hexutil.Uint(len(block.Uncles()))
		return &n
	}
	return nil
}

func (s *PublicBlockChainAPI) GetCode(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (hexutil.Bytes, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	code := state.GetCode(address)
	return code, state.Error()
}

func (s *PublicBlockChainAPI) GetStorageAt(ctx context.Context, address common.Address, key string, blockNr rpc.BlockNumber) (hexutil.Bytes, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	res := state.GetState(address, common.HexToHash(key))
	return res[:], state.Error()
}

type CallArgs struct {
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Gas      hexutil.Uint64  `json:"gas"`
	GasPrice hexutil.Big     `json:"gasPrice"`
	Value    hexutil.Big     `json:"value"`
	Data     hexutil.Bytes   `json:"data"`
}

func (s *PublicBlockChainAPI) doCall(ctx context.Context, args CallArgs, blockNr rpc.BlockNumber, vmCfg vm.Config, timeout time.Duration) (*core.ExecutionResult, error) {
	defer func(start time.Time) { log.Debug("Executing EVM call finished", "runtime", time.Since(start)) }(time.Now())

	state, header, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}

	addr := args.From
	if addr == (common.Address{}) {
		if wallets := s.b.AccountManager().Wallets(); len(wallets) > 0 {
			if accounts := wallets[0].Accounts(); len(accounts) > 0 {
				addr = accounts[0].Address
			}
		}
	}

	gas, gasPrice := uint64(args.Gas), args.GasPrice.ToInt()
	if gas == 0 {
		gas = math.MaxUint64 / 2
	}
	if gasPrice.Sign() == 0 {
		gasPrice = new(big.Int).SetUint64(defaultGasPrice)
	}

	msg := types.NewMessage(addr, args.To, 0, args.Value.ToInt(), gas, gasPrice, args.Data, false)

	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	defer cancel()

	evm, vmError, err := s.b.GetEVM(ctx, msg, state, header, vmCfg)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		evm.Cancel()
	}()

	gp := new(core.GasPool).AddGas(math.MaxUint64)
	result, _, err := core.ApplyMessageEx(evm, msg, gp)
	if err := vmError(); err != nil {
		return nil, err
	}

	if err != nil {
		return result, fmt.Errorf("err: %w (supplied gas %d)", err, msg.Gas())
	}

	return result, err
}

func newRevertError(result *core.ExecutionResult) *revertError {
	reason, errUnpack := abi.UnpackRevert(result.Revert())
	err := errors.New("execution reverted")
	if errUnpack == nil {
		err = fmt.Errorf("execution reverted: %v", reason)
	}
	return &revertError{
		error:  err,
		reason: hexutil.Encode(result.Revert()),
	}
}

type revertError struct {
	error
	reason string
}

func (e *revertError) ErrorCode() int {
	return 3
}

func (e *revertError) ErrorData() interface{} {
	return e.reason
}

func (s *PublicBlockChainAPI) Call(ctx context.Context, args CallArgs, blockNr rpc.BlockNumber) (hexutil.Bytes, error) {
	result, err := s.doCall(ctx, args, blockNr, vm.Config{}, 5*time.Second)

	if err != nil {
		return nil, err
	}

	if len(result.Revert()) > 0 {
		return nil, newRevertError(result)
	}
	return result.Return(), result.Err
}

func (s *PublicBlockChainAPI) EstimateGas(ctx context.Context, args CallArgs) (hexutil.Uint64, error) {

	var (
		lo  uint64 = params.TxGas - 1
		hi  uint64
		cap uint64
	)

	functionType, e := neatAbi.FunctionTypeFromId(args.Data[:4])
	if e == nil && functionType != neatAbi.Unknown {
		fmt.Printf("neatio inner contract tx, address: %v, functionType: %v\n", args.To.Hex(), functionType)
		return hexutil.Uint64(functionType.RequiredGas()), nil
	}

	if uint64(args.Gas) >= params.TxGas {
		hi = uint64(args.Gas)
	} else {

		block, err := s.b.BlockByNumber(ctx, rpc.PendingBlockNumber)
		if err != nil {
			return 0, err
		}
		hi = block.GasLimit()
	}
	cap = hi

	executable := func(gas uint64) (bool, *core.ExecutionResult, error) {
		args.Gas = hexutil.Uint64(gas)

		result, err := s.doCall(ctx, args, rpc.PendingBlockNumber, vm.Config{}, 0)

		if err != nil {
			if errors.Is(err, core.ErrIntrinsicGas) {
				return true, nil, nil
			}
			return true, nil, err
		}
		return result.Failed(), result, nil
	}

	for lo+1 < hi {

		mid := (hi + lo) / 2
		failed, _, err := executable(mid)

		if err != nil {
			return 0, err
		}
		if failed {
			lo = mid
		} else {
			hi = mid
		}
	}

	if hi == cap {

		failed, result, err := executable(hi)
		if err != nil {
			return 0, err
		}
		if failed {
			if result != nil && result.Err != vm.ErrOutOfGas {
				if len(result.Revert()) > 0 {
					return 0, newRevertError(result)
				}
				return 0, result.Err
			}

			return 0, fmt.Errorf("gas required exceeds allowance (%d)", cap)
		}
	}
	return hexutil.Uint64(hi), nil
}

type ExecutionResult struct {
	Gas         uint64         `json:"gas"`
	Failed      bool           `json:"failed"`
	ReturnValue string         `json:"returnValue"`
	StructLogs  []StructLogRes `json:"structLogs"`
}

type StructLogRes struct {
	Pc      uint64             `json:"pc"`
	Op      string             `json:"op"`
	Gas     uint64             `json:"gas"`
	GasCost uint64             `json:"gasCost"`
	Depth   int                `json:"depth"`
	Error   error              `json:"error,omitempty"`
	Stack   *[]string          `json:"stack,omitempty"`
	Memory  *[]string          `json:"memory,omitempty"`
	Storage *map[string]string `json:"storage,omitempty"`
}

func FormatLogs(logs []vm.StructLog) []StructLogRes {
	formatted := make([]StructLogRes, len(logs))
	for index, trace := range logs {
		formatted[index] = StructLogRes{
			Pc:      trace.Pc,
			Op:      trace.Op.String(),
			Gas:     trace.Gas,
			GasCost: trace.GasCost,
			Depth:   trace.Depth,
			Error:   trace.Err,
		}
		if trace.Stack != nil {
			stack := make([]string, len(trace.Stack))
			for i, stackValue := range trace.Stack {
				stack[i] = fmt.Sprintf("%x", math.PaddedBigBytes(stackValue, 32))
			}
			formatted[index].Stack = &stack
		}
		if trace.Memory != nil {
			memory := make([]string, 0, (len(trace.Memory)+31)/32)
			for i := 0; i+32 <= len(trace.Memory); i += 32 {
				memory = append(memory, fmt.Sprintf("%x", trace.Memory[i:i+32]))
			}
			formatted[index].Memory = &memory
		}
		if trace.Storage != nil {
			storage := make(map[string]string)
			for i, storageValue := range trace.Storage {
				storage[fmt.Sprintf("%x", i)] = fmt.Sprintf("%x", storageValue)
			}
			formatted[index].Storage = &storage
		}
	}
	return formatted
}

func (s *PublicBlockChainAPI) rpcOutputBlock(b *types.Block, inclTx bool, fullTx bool) (map[string]interface{}, error) {
	head := b.Header()
	fields := map[string]interface{}{
		"number": (*hexutil.Big)(head.Number),

		"hash":             b.Hash(),
		"parentHash":       head.ParentHash,
		"nonce":            head.Nonce,
		"mixHash":          head.MixDigest,
		"sha3Uncles":       head.UncleHash,
		"logsBloom":        head.Bloom,
		"stateRoot":        head.Root,
		"miner":            head.Coinbase,
		"difficulty":       (*hexutil.Big)(head.Difficulty),
		"totalDifficulty":  (*hexutil.Big)(s.b.GetTd(b.Hash())),
		"extraData":        hexutil.Bytes(head.Extra),
		"size":             hexutil.Uint64(b.Size()),
		"gasLimit":         hexutil.Uint64(head.GasLimit),
		"gasUsed":          hexutil.Uint64(head.GasUsed),
		"timestamp":        (*hexutil.Big)(head.Time),
		"transactionsRoot": head.TxHash,
		"receiptsRoot":     head.ReceiptHash,
	}

	if inclTx {
		formatTx := func(tx *types.Transaction) (interface{}, error) {
			return tx.Hash(), nil
		}

		if fullTx {
			formatTx = func(tx *types.Transaction) (interface{}, error) {
				return newRPCTransactionFromBlockHash(b, tx.Hash()), nil
			}
		}

		txs := b.Transactions()
		transactions := make([]interface{}, len(txs))
		var err error
		for i, tx := range b.Transactions() {
			if transactions[i], err = formatTx(tx); err != nil {
				return nil, err
			}
		}
		fields["transactions"] = transactions
	}

	uncles := b.Uncles()
	uncleHashes := make([]common.Hash, len(uncles))
	for i, uncle := range uncles {
		uncleHashes[i] = uncle.Hash()
	}
	fields["uncles"] = uncleHashes

	return fields, nil
}

type RPCTransaction struct {
	BlockHash        common.Hash     `json:"blockHash"`
	BlockNumber      *hexutil.Big    `json:"blockNumber"`
	From             common.Address  `json:"from"`
	Gas              hexutil.Uint64  `json:"gas"`
	GasPrice         *hexutil.Big    `json:"gasPrice"`
	Hash             common.Hash     `json:"hash"`
	Input            hexutil.Bytes   `json:"input"`
	Nonce            hexutil.Uint64  `json:"nonce"`
	To               *common.Address `json:"to"`
	TransactionIndex hexutil.Uint    `json:"transactionIndex"`
	Value            *hexutil.Big    `json:"value"`
	V                *hexutil.Big    `json:"v"`
	R                *hexutil.Big    `json:"r"`
	S                *hexutil.Big    `json:"s"`
}

func newRPCTransaction(tx *types.Transaction, blockHash common.Hash, blockNumber uint64, index uint64) *RPCTransaction {
	var signer types.Signer = types.FrontierSigner{}
	if tx.Protected() {
		signer = types.NewEIP155Signer(tx.ChainId())
	}
	from, _ := types.Sender(signer, tx)
	v, r, s := tx.RawSignatureValues()

	result := &RPCTransaction{
		From:     from,
		Gas:      hexutil.Uint64(tx.Gas()),
		GasPrice: (*hexutil.Big)(tx.GasPrice()),
		Hash:     tx.Hash(),
		Input:    hexutil.Bytes(tx.Data()),
		Nonce:    hexutil.Uint64(tx.Nonce()),
		To:       tx.To(),
		Value:    (*hexutil.Big)(tx.Value()),
		V:        (*hexutil.Big)(v),
		R:        (*hexutil.Big)(r),
		S:        (*hexutil.Big)(s),
	}
	if blockHash != (common.Hash{}) {
		result.BlockHash = blockHash
		result.BlockNumber = (*hexutil.Big)(new(big.Int).SetUint64(blockNumber))
		result.TransactionIndex = hexutil.Uint(index)
	}
	return result
}

func newRPCPendingTransaction(tx *types.Transaction) *RPCTransaction {
	return newRPCTransaction(tx, common.Hash{}, 0, 0)
}

func newRPCTransactionFromBlockIndex(b *types.Block, index uint64) *RPCTransaction {
	txs := b.Transactions()
	if index >= uint64(len(txs)) {
		return nil
	}
	return newRPCTransaction(txs[index], b.Hash(), b.NumberU64(), index)
}

func newRPCRawTransactionFromBlockIndex(b *types.Block, index uint64) hexutil.Bytes {
	txs := b.Transactions()
	if index >= uint64(len(txs)) {
		return nil
	}
	blob, _ := rlp.EncodeToBytes(txs[index])
	return blob
}

func newRPCTransactionFromBlockHash(b *types.Block, hash common.Hash) *RPCTransaction {
	for idx, tx := range b.Transactions() {
		if tx.Hash() == hash {
			return newRPCTransactionFromBlockIndex(b, uint64(idx))
		}
	}
	return nil
}

type PublicTransactionPoolAPI struct {
	b         Backend
	nonceLock *AddrLocker
}

func NewPublicTransactionPoolAPI(b Backend, nonceLock *AddrLocker) *PublicTransactionPoolAPI {
	return &PublicTransactionPoolAPI{b, nonceLock}
}

func (s *PublicTransactionPoolAPI) GetBlockTransactionCountByNumber(ctx context.Context, blockNr rpc.BlockNumber) *hexutil.Uint {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		n := hexutil.Uint(len(block.Transactions()))
		return &n
	}
	return nil
}

func (s *PublicTransactionPoolAPI) GetBlockTransactionCountByHash(ctx context.Context, blockHash common.Hash) *hexutil.Uint {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		n := hexutil.Uint(len(block.Transactions()))
		return &n
	}
	return nil
}

func (s *PublicTransactionPoolAPI) GetTransactionByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint) *RPCTransaction {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		return newRPCTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

func (s *PublicTransactionPoolAPI) GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) *RPCTransaction {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		return newRPCTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

func (s *PublicTransactionPoolAPI) GetRawTransactionByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint) hexutil.Bytes {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		return newRPCRawTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

func (s *PublicTransactionPoolAPI) GetRawTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) hexutil.Bytes {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		return newRPCRawTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

func (s *PublicTransactionPoolAPI) GetTransactionCount(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (*hexutil.Uint64, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	nonce := state.GetNonce(address)
	return (*hexutil.Uint64)(&nonce), state.Error()
}

func (s *PublicTransactionPoolAPI) GetTransactionByHash(ctx context.Context, hash common.Hash) *RPCTransaction {

	if tx, blockHash, blockNumber, index := rawdb.ReadTransaction(s.b.ChainDb(), hash); tx != nil {
		return newRPCTransaction(tx, blockHash, blockNumber, index)
	}

	if tx := s.b.GetPoolTransaction(hash); tx != nil {
		return newRPCPendingTransaction(tx)
	}

	return nil
}

func (s *PublicTransactionPoolAPI) GetRawTransactionByHash(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	var tx *types.Transaction

	if tx, _, _, _ = rawdb.ReadTransaction(s.b.ChainDb(), hash); tx == nil {
		if tx = s.b.GetPoolTransaction(hash); tx == nil {

			return nil, nil
		}
	}

	return rlp.EncodeToBytes(tx)
}

type Log struct {
	Address string `json:"address" gencodec:"required"`

	Topics []common.Hash `json:"topics" gencodec:"required"`

	Data string `json:"data" gencodec:"required"`

	BlockNumber uint64 `json:"blockNumber"`

	TxHash common.Hash `json:"transactionHash" gencodec:"required"`

	TxIndex uint `json:"transactionIndex" gencodec:"required"`

	BlockHash common.Hash `json:"blockHash"`

	Index uint `json:"logIndex" gencodec:"required"`

	Removed bool `json:"removed"`
}

func (s *PublicTransactionPoolAPI) GetTransactionReceipt(ctx context.Context, hash common.Hash) (map[string]interface{}, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(s.b.ChainDb(), hash)
	if tx == nil {
		return nil, nil
	}
	receipts, err := s.b.GetReceipts(ctx, blockHash)
	if err != nil {
		return nil, err
	}
	if len(receipts) <= int(index) {
		return nil, nil
	}
	receipt := receipts[index]

	var signer types.Signer = types.FrontierSigner{}
	if tx.Protected() {
		signer = types.NewEIP155Signer(tx.ChainId())
	}
	from, _ := types.Sender(signer, tx)

	fields := map[string]interface{}{
		"blockHash":         blockHash,
		"blockNumber":       hexutil.Uint64(blockNumber),
		"transactionHash":   hash,
		"transactionIndex":  hexutil.Uint64(index),
		"from":              from,
		"to":                tx.To(),
		"gasUsed":           hexutil.Uint64(receipt.GasUsed),
		"cumulativeGasUsed": hexutil.Uint64(receipt.CumulativeGasUsed),
		"contractAddress":   nil,
		"logs":              receipt.Logs,
		"logsBloom":         receipt.Bloom,
	}

	if len(receipt.PostState) > 0 {
		fields["root"] = hexutil.Bytes(receipt.PostState)
	} else {
		fields["status"] = hexutil.Uint(receipt.Status)
	}
	if receipt.Logs == nil {
		fields["logs"] = [][]*types.Log{}
	}

	if receipt.ContractAddress != (common.Address{}) {
		fields["contractAddress"] = receipt.ContractAddress
	}
	return fields, nil
}

func (s *PublicTransactionPoolAPI) sign(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {

	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}

	var chainID *big.Int
	if config := s.b.ChainConfig(); config.IsEIP155(s.b.CurrentBlock().Number()) {
		chainID = config.ChainId
	}
	return wallet.SignTxWithAddress(account, tx, chainID)
}

type SendTxArgs struct {
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Nonce    *hexutil.Uint64 `json:"nonce"`

	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input"`
}

func (args *SendTxArgs) setDefaults(ctx context.Context, b Backend) error {

	var function = neatAbi.Unknown
	if neatAbi.IsNeatChainContractAddr(args.To) {
		var input []byte
		if args.Data != nil {
			input = *args.Data
		} else if args.Input != nil {
			input = *args.Input
		}
		if len(input) == 0 {
			return errors.New(`neatio contract without any data provided`)
		}

		var err error
		function, err = neatAbi.FunctionTypeFromId(input[:4])
		if err != nil {
			return err
		}
	}

	if function == neatAbi.DepositInSideChain || function == neatAbi.WithdrawFromMainChain || function == neatAbi.SaveDataToMainChain {
		args.Gas = new(hexutil.Uint64)
		*(*uint64)(args.Gas) = 0
	} else {
		if args.Gas == nil {
			args.Gas = new(hexutil.Uint64)
			*(*uint64)(args.Gas) = 90000
		}
	}

	if args.GasPrice == nil {
		price, err := b.SuggestPrice(ctx)
		if err != nil {
			return err
		}
		args.GasPrice = (*hexutil.Big)(price)
	}
	if args.Value == nil {
		args.Value = new(hexutil.Big)
	}
	if args.Data != nil && args.Input != nil && !bytes.Equal(*args.Data, *args.Input) {
		return errors.New(`Both "data" and "input" are set and not equal. Please use "input" to pass transaction call data.`)
	}
	if args.To == nil {

		var input []byte
		if args.Data != nil {
			input = *args.Data
		} else if args.Input != nil {
			input = *args.Input
		}
		if len(input) == 0 {
			return errors.New(`contract creation without any data provided`)
		}
	}

	if args.Nonce == nil {
		nonce, err := b.GetPoolNonce(ctx, args.From)
		if err != nil {
			return err
		}
		args.Nonce = (*hexutil.Uint64)(&nonce)
	}

	return nil
}

func (args *SendTxArgs) toTransaction() *types.Transaction {
	var input []byte
	if args.Data != nil {
		input = *args.Data
	} else if args.Input != nil {
		input = *args.Input
	}
	if args.To == nil {
		return types.NewContractCreation(uint64(*args.Nonce), (*big.Int)(args.Value), uint64(*args.Gas), (*big.Int)(args.GasPrice), input)
	}
	return types.NewTransaction(uint64(*args.Nonce), *args.To, (*big.Int)(args.Value), uint64(*args.Gas), (*big.Int)(args.GasPrice), input)
}

func submitTransaction(ctx context.Context, b Backend, tx *types.Transaction) (common.Hash, error) {
	if err := b.SendTx(ctx, tx); err != nil {
		return common.Hash{}, err
	}
	if tx.To() == nil {
		signer := types.MakeSigner(b.ChainConfig(), b.CurrentBlock().Number())
		from, err := types.Sender(signer, tx)
		if err != nil {
			return common.Hash{}, err
		}
		addr := crypto.CreateAddress(from, tx.Nonce())
		log.Info("Submitted contract creation", "fullhash", tx.Hash().Hex(), "contract", addr.Hex())
	} else {
		log.Info("Submitted transaction", "fullhash", tx.Hash().Hex(), "recipient", tx.To())
	}
	return tx.Hash(), nil
}

func (s *PublicTransactionPoolAPI) SendTransaction(ctx context.Context, args SendTxArgs) (common.Hash, error) {
	fmt.Printf("transaction args PublicTransactionPoolAPI args %v\n", args)

	account := accounts.Account{Address: args.From}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return common.Hash{}, err
	}

	if args.Nonce == nil {

		s.nonceLock.LockAddr(args.From)
		defer s.nonceLock.UnlockAddr(args.From)
	}

	if err := args.setDefaults(ctx, s.b); err != nil {
		return common.Hash{}, err
	}

	tx := args.toTransaction()

	var chainID *big.Int
	if config := s.b.ChainConfig(); config.IsEIP155(s.b.CurrentBlock().Number()) {
		chainID = config.ChainId
	}
	signed, err := wallet.SignTxWithAddress(account, tx, chainID)
	if err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, s.b, signed)
}

func SendTransaction(ctx context.Context, args SendTxArgs, am *accounts.Manager, b Backend, nonceLock *AddrLocker) (common.Hash, error) {
	fmt.Printf("transaction args PublicTransactionPoolAPI args %v\n", args)

	account := accounts.Account{Address: args.From}

	wallet, err := am.Find(account)
	if err != nil {
		return common.Hash{}, err
	}

	if args.Nonce == nil {

		nonceLock.LockAddr(args.From)
		defer nonceLock.UnlockAddr(args.From)
	}

	if err := args.setDefaults(ctx, b); err != nil {
		return common.Hash{}, err
	}

	tx := args.toTransaction()

	var chainID *big.Int
	if config := b.ChainConfig(); config.IsEIP155(b.CurrentBlock().Number()) {
		chainID = config.ChainId
	}
	signed, err := wallet.SignTxWithAddress(account, tx, chainID)
	if err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, b, signed)
}

func (s *PublicTransactionPoolAPI) SendRawTransaction(ctx context.Context, encodedTx hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := rlp.DecodeBytes(encodedTx, tx); err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, s.b, tx)
}

func (s *PublicTransactionPoolAPI) Sign(addr common.Address, data hexutil.Bytes) (hexutil.Bytes, error) {

	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}

	signature, err := wallet.SignHash(account, signHash(data))
	if err == nil {
		signature[64] += 27
	}
	return signature, err
}

type SignTransactionResult struct {
	Raw hexutil.Bytes      `json:"raw"`
	Tx  *types.Transaction `json:"tx"`
}

func (s *PublicTransactionPoolAPI) SignTransaction(ctx context.Context, args SendTxArgs) (*SignTransactionResult, error) {
	if args.Gas == nil {
		return nil, fmt.Errorf("gas not specified")
	}
	if args.GasPrice == nil {
		return nil, fmt.Errorf("gasPrice not specified")
	}
	if args.Nonce == nil {
		return nil, fmt.Errorf("nonce not specified")
	}
	if err := args.setDefaults(ctx, s.b); err != nil {
		return nil, err
	}
	tx, err := s.sign(args.From, args.toTransaction())
	if err != nil {
		return nil, err
	}
	data, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return nil, err
	}
	return &SignTransactionResult{data, tx}, nil
}

func (s *PublicTransactionPoolAPI) PendingTransactions() ([]*RPCTransaction, error) {
	pending, err := s.b.GetPoolTransactions()
	if err != nil {
		return nil, err
	}

	transactions := make([]*RPCTransaction, 0, len(pending))
	for _, tx := range pending {
		var signer types.Signer = types.HomesteadSigner{}
		if tx.Protected() {
			signer = types.NewEIP155Signer(tx.ChainId())
		}
		from, _ := types.Sender(signer, tx)
		if _, err := s.b.AccountManager().Find(accounts.Account{Address: from}); err == nil {
			transactions = append(transactions, newRPCPendingTransaction(tx))
		}
	}
	return transactions, nil
}

func (s *PublicTransactionPoolAPI) Resend(ctx context.Context, sendArgs SendTxArgs, gasPrice *hexutil.Big, gasLimit *hexutil.Uint64) (common.Hash, error) {
	if sendArgs.Nonce == nil {
		return common.Hash{}, fmt.Errorf("missing transaction nonce in transaction spec")
	}
	if err := sendArgs.setDefaults(ctx, s.b); err != nil {
		return common.Hash{}, err
	}
	matchTx := sendArgs.toTransaction()
	pending, err := s.b.GetPoolTransactions()
	if err != nil {
		return common.Hash{}, err
	}

	for _, p := range pending {
		var signer types.Signer = types.HomesteadSigner{}
		if p.Protected() {
			signer = types.NewEIP155Signer(p.ChainId())
		}
		wantSigHash := signer.Hash(matchTx)

		if pFrom, err := types.Sender(signer, p); err == nil && pFrom == sendArgs.From && signer.Hash(p) == wantSigHash {

			if gasPrice != nil && (*big.Int)(gasPrice).Sign() != 0 {
				sendArgs.GasPrice = gasPrice
			}
			if gasLimit != nil && *gasLimit != 0 {
				sendArgs.Gas = gasLimit
			}
			signedTx, err := s.sign(sendArgs.From, sendArgs.toTransaction())
			if err != nil {
				return common.Hash{}, err
			}
			if err = s.b.SendTx(ctx, signedTx); err != nil {
				return common.Hash{}, err
			}
			return signedTx.Hash(), nil
		}
	}

	return common.Hash{}, fmt.Errorf("Transaction %#x not found", matchTx.Hash())
}

type PublicDebugAPI struct {
	b Backend
}

func NewPublicDebugAPI(b Backend) *PublicDebugAPI {
	return &PublicDebugAPI{b: b}
}

func (api *PublicDebugAPI) GetBlockRlp(ctx context.Context, number uint64) (string, error) {
	block, _ := api.b.BlockByNumber(ctx, rpc.BlockNumber(number))
	if block == nil {
		return "", fmt.Errorf("block #%d not found", number)
	}
	encoded, err := rlp.EncodeToBytes(block)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", encoded), nil
}

func (api *PublicDebugAPI) PrintBlock(ctx context.Context, number uint64) (string, error) {
	block, _ := api.b.BlockByNumber(ctx, rpc.BlockNumber(number))
	if block == nil {
		return "", fmt.Errorf("block #%d not found", number)
	}
	return block.String(), nil
}

type PrivateDebugAPI struct {
	b Backend
}

func NewPrivateDebugAPI(b Backend) *PrivateDebugAPI {
	return &PrivateDebugAPI{b: b}
}

func (api *PrivateDebugAPI) ChaindbProperty(property string) (string, error) {
	ldb, ok := api.b.ChainDb().(interface {
		LDB() *leveldb.DB
	})
	if !ok {
		return "", fmt.Errorf("chaindbProperty does not work for memory databases")
	}
	if property == "" {
		property = "leveldb.stats"
	} else if !strings.HasPrefix(property, "leveldb.") {
		property = "leveldb." + property
	}
	return ldb.LDB().GetProperty(property)
}

func (api *PrivateDebugAPI) ChaindbCompact() error {
	for b := byte(0); b < 255; b++ {
		log.Info("Compacting chain database", "range", fmt.Sprintf("0x%0.2X-0x%0.2X", b, b+1))
		if err := api.b.ChainDb().Compact([]byte{b}, []byte{b + 1}); err != nil {
			log.Error("Database compaction failed", "err", err)
			return err
		}
	}
	return nil
}

func (api *PrivateDebugAPI) SetHead(number hexutil.Uint64) {
	api.b.SetHead(uint64(number))
}

type PublicNetAPI struct {
	net            *p2p.Server
	networkVersion uint64
}

func NewPublicNetAPI(net *p2p.Server, networkVersion uint64) *PublicNetAPI {
	return &PublicNetAPI{net, networkVersion}
}

func (s *PublicNetAPI) Listening() bool {
	return true
}

func (s *PublicNetAPI) PeerCount() hexutil.Uint {
	return hexutil.Uint(s.net.PeerCount())
}

func (s *PublicNetAPI) Version() string {
	return fmt.Sprintf("%d", s.networkVersion)
}

type PublicNEATAPI struct {
	am        *accounts.Manager
	b         Backend
	nonceLock *AddrLocker
}

func NewPublicNEATAPI(b Backend, nonceLock *AddrLocker) *PublicNEATAPI {
	return &PublicNEATAPI{b.AccountManager(), b, nonceLock}
}

func (s *PublicNEATAPI) SignAddress(from common.Address, consensusPrivateKey hexutil.Bytes) (goCrypto.Signature, error) {
	if len(consensusPrivateKey) != 32 {
		return nil, errors.New("invalid consensus private key")
	}

	var blsPriv goCrypto.BLSPrivKey
	copy(blsPriv[:], consensusPrivateKey)

	blsSign := blsPriv.Sign(from.Bytes())

	return blsSign, nil
}

func (api *PublicNEATAPI) WithdrawReward(ctx context.Context, from common.Address, delegateAddress common.Address, amount *hexutil.Big, gasPrice *hexutil.Big) (common.Hash, error) {
	input, err := neatAbi.ChainABI.Pack(neatAbi.WithdrawReward.String(), delegateAddress, (*big.Int)(amount))
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.WithdrawReward.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}

	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) Delegate(ctx context.Context, from, candidate common.Address, amount *hexutil.Big, gasPrice *hexutil.Big) (common.Hash, error) {

	input, err := neatAbi.ChainABI.Pack(neatAbi.Delegate.String(), candidate)
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.Delegate.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    amount,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}
	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) UnDelegate(ctx context.Context, from, candidate common.Address, amount *hexutil.Big, gasPrice *hexutil.Big) (common.Hash, error) {

	input, err := neatAbi.ChainABI.Pack(neatAbi.UnDelegate.String(), candidate, (*big.Int)(amount))
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.UnDelegate.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}

	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) Register(ctx context.Context, from common.Address, registerAmount *hexutil.Big, pubkey goCrypto.BLSPubKey, signature hexutil.Bytes, commission uint8, gasPrice *hexutil.Big) (common.Hash, error) {

	input, err := neatAbi.ChainABI.Pack(neatAbi.Register.String(), pubkey.Bytes(), signature, commission)
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.Register.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    registerAmount,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}
	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) UnRegister(ctx context.Context, from common.Address, gasPrice *hexutil.Big) (common.Hash, error) {

	input, err := neatAbi.ChainABI.Pack(neatAbi.UnRegister.String())
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.UnRegister.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}
	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) CheckCandidate(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[string]interface{}, error) {
	state, _, err := api.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}

	fields := map[string]interface{}{
		"candidate":  state.IsCandidate(address),
		"commission": state.GetCommission(address),
	}
	return fields, state.Error()
}

func (api *PublicNEATAPI) SetCommission(ctx context.Context, from common.Address, commission uint8, gasPrice *hexutil.Big) (common.Hash, error) {
	input, err := neatAbi.ChainABI.Pack(neatAbi.SetCommission.String(), commission)
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.SetCommission.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}

	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) EditValidator(ctx context.Context, from common.Address, moniker, website string, identity string, details string, gasPrice *hexutil.Big) (common.Hash, error) {
	input, err := neatAbi.ChainABI.Pack(neatAbi.EditValidator.String(), moniker, website, identity, details)
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.EditValidator.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}

	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) SetAddress(ctx context.Context, from, fAddress common.Address, gasPrice *hexutil.Big) (common.Hash, error) {
	input, err := neatAbi.ChainABI.Pack(neatAbi.SetAddress.String(), fAddress)
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.SetAddress.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.NeatioSmartContractAddress,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}

	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func init() {

	core.RegisterValidateCb(neatAbi.WithdrawReward, withdrawRewardValidateCb)
	core.RegisterApplyCb(neatAbi.WithdrawReward, withdrawRewardApplyCb)

	core.RegisterValidateCb(neatAbi.Delegate, delegateValidateCb)
	core.RegisterApplyCb(neatAbi.Delegate, delegateApplyCb)

	core.RegisterValidateCb(neatAbi.UnDelegate, unDelegateValidateCb)
	core.RegisterApplyCb(neatAbi.UnDelegate, unDelegateApplyCb)

	core.RegisterValidateCb(neatAbi.Register, registerValidateCb)
	core.RegisterApplyCb(neatAbi.Register, registerApplyCb)

	core.RegisterValidateCb(neatAbi.UnRegister, unRegisterValidateCb)
	core.RegisterApplyCb(neatAbi.UnRegister, unRegisterApplyCb)

	core.RegisterValidateCb(neatAbi.SetCommission, setCommisstionValidateCb)
	core.RegisterApplyCb(neatAbi.SetCommission, setCommisstionApplyCb)

	core.RegisterValidateCb(neatAbi.EditValidator, editValidatorValidateCb)

	core.RegisterValidateCb(neatAbi.SetAddress, setAddressValidateCb)
	core.RegisterApplyCb(neatAbi.SetAddress, setAddressApplyCb)
}

func withdrawRewardValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	_, err := withDrawRewardValidation(from, tx, state, bc)
	if err != nil {
		return err
	}

	return nil
}

func withdrawRewardApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {
	from := derivedAddressFromTx(tx)

	args, err := withDrawRewardValidation(from, tx, state, bc)
	if err != nil {
		return err
	}

	state.SubRewardBalanceByDelegateAddress(from, args.DelegateAddress, args.Amount)
	state.AddBalance(from, args.Amount)

	return nil
}

func withDrawRewardValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.WithdrawRewardArgs, error) {

	var args neatAbi.WithdrawRewardArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.WithdrawReward.String(), data[4:]); err != nil {
		return nil, err
	}

	reward := state.GetRewardBalanceByDelegateAddress(from, args.DelegateAddress)

	if reward.Sign() < 1 {
		return nil, fmt.Errorf("have no reward to withdraw")
	}

	if args.Amount.Sign() == -1 {
		return nil, fmt.Errorf("widthdraw amount can not be negative")
	}

	if args.Amount.Cmp(reward) == 1 {
		return nil, fmt.Errorf("reward balance not enough, withdraw amount %v, but balance %v, delegate address %v", args.Amount, reward, args.DelegateAddress)
	}
	return &args, nil
}

func registerValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	_, verror := registerValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}
	return nil
}

func registerApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {

	from := derivedAddressFromTx(tx)
	args, verror := registerValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}

	verror = updateValidation(bc)
	if verror != nil {
		return verror
	}

	amount := tx.Value()

	state.SubBalance(from, amount)
	state.AddDelegateBalance(from, amount)
	state.AddProxiedBalanceByUser(from, from, amount)

	var blsPK goCrypto.BLSPubKey
	copy(blsPK[:], args.Pubkey)
	if verror != nil {
		return verror
	}
	state.ApplyForCandidate(from, blsPK.KeyString(), args.Commission)

	verror = updateNextEpochValidatorVoteSet(tx, state, bc, from, ops)
	if verror != nil {
		return verror
	}

	return nil
}

func registerValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.RegisterArgs, error) {

	if !state.IsCleanAddress(from) {
		return nil, core.ErrAlreadyCandidate
	}

	if tx.Value().Cmp(minimumRegisterAmount) == -1 {
		return nil, core.ErrMinimumRegisterAmount
	}

	var args neatAbi.RegisterArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.Register.String(), data[4:]); err != nil {
		return nil, err
	}

	if err := goCrypto.CheckConsensusPubKey(from, args.Pubkey, args.Signature); err != nil {
		return nil, err
	}

	if args.Commission > 100 {
		return nil, core.ErrCommission
	}

	var ep *epoch.Epoch
	if nc, ok := bc.Engine().(consensus.NeatCon); ok {
		ep = nc.GetEpoch().GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
	}
	if _, supernode := ep.Validators.GetByAddress(from.Bytes()); supernode != nil && supernode.RemainingEpoch > 0 {
		return nil, core.ErrCannotCandidate
	}

	return &args, nil
}

func unRegisterValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	verror := unRegisterValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}
	return nil
}

func unRegisterApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {

	from := derivedAddressFromTx(tx)
	verror := unRegisterValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}

	allRefund := true

	state.ForEachProxied(from, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {

		state.SubProxiedBalanceByUser(from, key, proxiedBalance)
		state.SubDelegateBalance(key, proxiedBalance)
		state.AddBalance(key, proxiedBalance)

		if depositProxiedBalance.Sign() > 0 {
			allRefund = false

			refunded := state.GetPendingRefundBalanceByUser(from, key)

			state.AddPendingRefundBalanceByUser(from, key, new(big.Int).Sub(depositProxiedBalance, refunded))

			state.MarkDelegateAddressRefund(from)
		}
		return true
	})

	state.CancelCandidate(from, allRefund)

	return nil
}

func unRegisterValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {

	if !state.IsCandidate(from) {
		return core.ErrNotCandidate
	}

	var ep *epoch.Epoch
	if nc, ok := bc.Engine().(consensus.NeatCon); ok {
		ep = nc.GetEpoch().GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
	}
	if _, supernode := ep.Validators.GetByAddress(from.Bytes()); supernode != nil && supernode.RemainingEpoch > 0 {
		return core.ErrCannotUnRegister
	}

	if _, err := getEpoch(bc); err != nil {
		return err
	}

	return nil
}

func delegateValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	_, verror := delegateValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}
	return nil
}

func delegateApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {

	from := derivedAddressFromTx(tx)
	args, verror := delegateValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}

	verror = updateValidation(bc)
	if verror != nil {
		return verror
	}

	amount := tx.Value()

	state.SubBalance(from, amount)
	state.AddDelegateBalance(from, amount)

	state.AddProxiedBalanceByUser(args.Candidate, from, amount)

	verror = updateNextEpochValidatorVoteSet(tx, state, bc, args.Candidate, ops)
	if verror != nil {
		return verror
	}

	return nil
}

func delegateValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.DelegateArgs, error) {

	if tx.Value().Sign() == -1 {
		return nil, core.ErrDelegateAmount
	}

	var args neatAbi.DelegateArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.Delegate.String(), data[4:]); err != nil {
		return nil, err
	}

	if !state.IsCandidate(args.Candidate) {
		return nil, core.ErrNotCandidate
	}

	depositBalance := state.GetDepositProxiedBalanceByUser(args.Candidate, from)
	if depositBalance.Sign() == 0 {

		delegatedAddressNumber := state.GetProxiedAddressNumber(args.Candidate)
		if delegatedAddressNumber >= maxDelegationAddresses {
			return nil, core.ErrExceedDelegationAddressLimit
		}
	}

	var ep *epoch.Epoch
	if nc, ok := bc.Engine().(consensus.NeatCon); ok {
		ep = nc.GetEpoch().GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
	}
	if _, supernode := ep.Validators.GetByAddress(args.Candidate.Bytes()); supernode != nil && supernode.RemainingEpoch > 0 {
		if depositBalance.Sign() == 0 {
			return nil, core.ErrCannotDelegate
		}
	}

	return &args, nil
}

func unDelegateValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	_, verror := unDelegateValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}
	return nil
}

func unDelegateApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {

	from := derivedAddressFromTx(tx)
	args, verror := unDelegateValidation(from, tx, state, bc)
	if verror != nil {
		return verror
	}

	verror = updateValidation(bc)
	if verror != nil {
		return verror
	}

	proxiedBalance := state.GetProxiedBalanceByUser(args.Candidate, from)
	var immediatelyRefund *big.Int
	if args.Amount.Cmp(proxiedBalance) <= 0 {
		immediatelyRefund = args.Amount
	} else {
		immediatelyRefund = proxiedBalance
		restRefund := new(big.Int).Sub(args.Amount, proxiedBalance)
		state.AddPendingRefundBalanceByUser(args.Candidate, from, restRefund)

		state.MarkDelegateAddressRefund(args.Candidate)
	}

	state.SubProxiedBalanceByUser(args.Candidate, from, immediatelyRefund)
	state.SubDelegateBalance(from, immediatelyRefund)
	state.AddBalance(from, immediatelyRefund)

	verror = updateNextEpochValidatorVoteSet(tx, state, bc, args.Candidate, ops)
	if verror != nil {
		return verror
	}

	return nil
}

func unDelegateValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.UnDelegateArgs, error) {

	var args neatAbi.UnDelegateArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.UnDelegate.String(), data[4:]); err != nil {
		return nil, err
	}

	if args.Amount.Sign() == -1 {
		return nil, fmt.Errorf("undelegate amount can not be negative")
	}

	if from == args.Candidate {
		return nil, core.ErrCancelSelfDelegate
	}

	var ep *epoch.Epoch
	if nc, ok := bc.Engine().(consensus.NeatCon); ok {
		ep = nc.GetEpoch().GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
	}
	if _, supernode := ep.Validators.GetByAddress(args.Candidate.Bytes()); supernode != nil && supernode.RemainingEpoch > 0 {
		return nil, core.ErrCannotUnBond
	}

	proxiedBalance := state.GetProxiedBalanceByUser(args.Candidate, from)
	depositProxiedBalance := state.GetDepositProxiedBalanceByUser(args.Candidate, from)
	pendingRefundBalance := state.GetPendingRefundBalanceByUser(args.Candidate, from)

	netDeposit := new(big.Int).Sub(depositProxiedBalance, pendingRefundBalance)

	availableRefundBalance := new(big.Int).Add(proxiedBalance, netDeposit)
	if args.Amount.Cmp(availableRefundBalance) == 1 {
		return nil, core.ErrInsufficientProxiedBalance
	}

	if _, err := getEpoch(bc); err != nil {
		return nil, err
	}

	return &args, nil
}

func setCommisstionValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	_, err := setCommissionValidation(from, tx, state, bc)
	if err != nil {
		return err
	}

	return nil
}

func setCommisstionApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {
	from := derivedAddressFromTx(tx)
	args, err := setCommissionValidation(from, tx, state, bc)
	if err != nil {
		return err
	}

	state.SetCommission(from, args.Commission)

	return nil
}

func setCommissionValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.SetCommissionArgs, error) {
	if !state.IsCandidate(from) {
		return nil, core.ErrNotCandidate
	}

	var args neatAbi.SetCommissionArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.SetCommission.String(), data[4:]); err != nil {
		return nil, err
	}

	if args.Commission > 100 {
		return nil, core.ErrCommission
	}

	return &args, nil
}

func setAddressValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	_, err := setAddressValidation(from, tx, state, bc)
	if err != nil {
		return err
	}

	return nil
}

func setAddressApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {
	from := derivedAddressFromTx(tx)
	args, err := setAddressValidation(from, tx, state, bc)
	if err != nil {
		return err
	}

	state.SetAddress(from, args.FAddress)

	return nil
}

func setAddressValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.SetAddressArgs, error) {
	var args neatAbi.SetAddressArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.SetAddress.String(), data[4:]); err != nil {
		return nil, err
	}

	return &args, nil
}

func editValidatorValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)
	if !state.IsCandidate(from) {
		return errors.New("you are not a validator or candidate")
	}

	var args neatAbi.EditValidatorArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.EditValidator.String(), data[4:]); err != nil {
		return err
	}

	if len([]byte(args.Details)) > maxEditValidatorLength ||
		len([]byte(args.Identity)) > maxEditValidatorLength ||
		len([]byte(args.Moniker)) > maxEditValidatorLength ||
		len([]byte(args.Website)) > maxEditValidatorLength {

		return fmt.Errorf("args length too long, more than %v", maxEditValidatorLength)
	}

	return nil
}

func concatCopyPreAllocate(slices [][]byte) []byte {
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	tmp := make([]byte, totalLen)
	var i int
	for _, s := range slices {
		i += copy(tmp[i:], s)
	}
	return tmp
}

func getEpoch(bc *core.BlockChain) (*epoch.Epoch, error) {
	var ep *epoch.Epoch
	if nc, ok := bc.Engine().(consensus.NeatCon); ok {
		ep = nc.GetEpoch().GetEpochByBlockNumber(bc.CurrentBlock().NumberU64())
	}

	if ep == nil {
		return nil, errors.New("epoch is nil, are you running on NeatCon Consensus Engine")
	}

	return ep, nil
}

func derivedAddressFromTx(tx *types.Transaction) (from common.Address) {
	signer := types.NewEIP155Signer(tx.ChainId())
	from, _ = types.Sender(signer, tx)
	return
}

func updateValidation(bc *core.BlockChain) error {
	ep, err := getEpoch(bc)
	if err != nil {
		return err
	}

	currHeight := bc.CurrentBlock().NumberU64()

	if currHeight <= ep.StartBlock+2 || currHeight == ep.EndBlock {
		return errors.New("incorrect block height, please retry later")
	}

	return nil
}

func updateNextEpochValidatorVoteSet(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, candidate common.Address, ops *types.PendingOps) error {
	var update bool
	ep, err := getEpoch(bc)
	if err != nil {
		return err
	}

	proxiedBalance := state.GetTotalProxiedBalance(candidate)
	depositProxiedBalance := state.GetTotalDepositProxiedBalance(candidate)
	pendingRefundBalance := state.GetTotalPendingRefundBalance(candidate)
	netProxied := new(big.Int).Sub(new(big.Int).Add(proxiedBalance, depositProxiedBalance), pendingRefundBalance)

	if netProxied.Sign() == -1 {
		return errors.New("validator voting power can not be negative")
	}

	currentEpochVoteSet := ep.GetEpochValidatorVoteSet()

	if currentEpochVoteSet == nil {
		update = true
	} else {

		if len(currentEpochVoteSet.Votes) >= updateValidatorThreshold {
			for _, val := range currentEpochVoteSet.Votes {

				if val.Amount.Cmp(netProxied) == -1 {
					update = true
					break
				}
			}
		} else {
			update = true
		}
	}

	if update && state.IsCandidate(candidate) {

		var pubkey string
		pubkey = state.GetPubkey(candidate)
		pubkeyBytes := common.FromHex(pubkey)
		if pubkey == "" || len(pubkeyBytes) != 128 {
			return errors.New("wrong format of required field 'pub_key'")
		}
		var blsPK goCrypto.BLSPubKey
		copy(blsPK[:], pubkeyBytes)

		op := types.UpdateNextEpochOp{
			From:   candidate,
			PubKey: blsPK,
			Amount: netProxied,
			Salt:   "neatio",
			TxHash: tx.Hash(),
		}

		if ok := ops.Append(&op); !ok {
			return fmt.Errorf("pending ops conflict: %v", op)
		}
	}

	return nil
}
