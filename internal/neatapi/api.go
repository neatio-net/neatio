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

	"github.com/neatlab/neatio/chain/accounts"
	"github.com/neatlab/neatio/chain/accounts/keystore"
	"github.com/neatlab/neatio/chain/core"
	"github.com/neatlab/neatio/chain/core/rawdb"
	"github.com/neatlab/neatio/chain/core/types"
	"github.com/neatlab/neatio/chain/core/vm"
	"github.com/neatlab/neatio/chain/log"
	neatAbi "github.com/neatlab/neatio/neatabi/abi"
	"github.com/neatlab/neatio/network/p2p"
	"github.com/neatlab/neatio/network/rpc"
	"github.com/neatlab/neatio/params"
	"github.com/neatlab/neatio/utilities/common"
	"github.com/neatlab/neatio/utilities/common/hexutil"
	"github.com/neatlab/neatio/utilities/common/math"
	"github.com/neatlab/neatio/utilities/crypto"
	"github.com/neatlab/neatio/utilities/rlp"
	goCrypto "github.com/neatlib/crypto-go"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	defaultGasPrice          = params.GWei
	updateValidatorThreshold = 1000
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

		content["pending"][account.String()] = dump
	}

	for account, txs := range queue {
		dump := make(map[string]*RPCTransaction)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = newRPCPendingTransaction(tx)
		}

		content["queued"][account.String()] = dump
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

			return fmt.Sprintf("%s: %v wei + %v gas × %v wei", tx.To().String(), tx.Value(), tx.Gas(), tx.GasPrice())
		}
		return fmt.Sprintf("contract creation: %v wei + %v gas × %v wei", tx.Value(), tx.Gas(), tx.GasPrice())
	}

	for account, txs := range pending {
		dump := make(map[string]string)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = format(tx)
		}

		content["pending"][account.String()] = dump
	}

	for account, txs := range queue {
		dump := make(map[string]string)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = format(tx)
		}

		content["queued"][account.String()] = dump
	}
	return content
}

type PublicAccountAPI struct {
	am *accounts.Manager
}

func NewPublicAccountAPI(am *accounts.Manager) *PublicAccountAPI {
	return &PublicAccountAPI{am: am}
}

func (s *PublicAccountAPI) Accounts() []string {
	addresses := make([]string, 0)
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address.String())
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

func (s *PrivateAccountAPI) ListAccounts() []string {
	addresses := make([]string, 0)
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address.String())
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

func (s *PrivateAccountAPI) NewAccount(password string) (string, error) {
	acc, err := fetchKeystore(s.am).NewAccount(password)
	if err == nil {
		return acc.Address.String(), nil
	}
	return "", err
}

func fetchKeystore(am *accounts.Manager) *keystore.KeyStore {
	return am.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
}

func (s *PrivateAccountAPI) ImportRawKey(privkey string, password string) (string, error) {
	key, err := crypto.HexToECDSA(privkey)
	if err != nil {
		return "", err
	}
	acc, err := fetchKeystore(s.am).ImportECDSA(key, password)
	return acc.Address.String(), err
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

func (s *PrivateAccountAPI) EcRecover(ctx context.Context, data, sig hexutil.Bytes) (string, error) {
	if len(sig) != 65 {
		return "", fmt.Errorf("signature must be 65 bytes long")
	}
	if sig[64] != 27 && sig[64] != 28 {
		return "", fmt.Errorf("invalid NEAT Blockchain signature (V is not 27 or 28)")
	}
	sig[64] -= 27

	rpk, err := crypto.SigToPub(signHash(data), sig)
	if err != nil {
		return "", err
	}
	return crypto.PubkeyToAddress(*rpk).String(), nil
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

func (s *PublicBlockChainAPI) GetCandidateSetByBlockNumber(ctx context.Context, blockNr rpc.BlockNumber) ([]string, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}

	var candidateList = make([]string, 0)

	for addr := range state.GetCandidateSet() {
		candidateList = append(candidateList, addr.String())
	}

	return candidateList, nil
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
		proxied_detail := make(map[string]struct {
			ProxiedBalance        *hexutil.Big
			DepositProxiedBalance *hexutil.Big
			PendingRefundBalance  *hexutil.Big
		})
		state.ForEachProxied(address, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
			proxied_detail[key.String()] = struct {
				ProxiedBalance        *hexutil.Big
				DepositProxiedBalance *hexutil.Big
				PendingRefundBalance  *hexutil.Big
			}{
				ProxiedBalance:        (*hexutil.Big)(proxiedBalance),
				DepositProxiedBalance: (*hexutil.Big)(depositProxiedBalance),
				PendingRefundBalance:  (*hexutil.Big)(pendingRefundBalance),
			}
			return true
		})

		fields["proxiedDetail"] = proxied_detail

		reward_detail := make(map[string]*hexutil.Big)
		state.ForEachReward(address, func(key common.Address, rewardBalance *big.Int) bool {
			reward_detail[key.String()] = (*hexutil.Big)(rewardBalance)
			return true
		})

		fields["rewardDetail"] = reward_detail
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

func (s *PublicBlockChainAPI) doCall(ctx context.Context, args CallArgs, blockNr rpc.BlockNumber, vmCfg vm.Config, timeout time.Duration) ([]byte, uint64, bool, error) {
	defer func(start time.Time) { log.Debug("Executing EVM call finished", "runtime", time.Since(start)) }(time.Now())

	state, header, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, 0, false, err
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
		return nil, 0, false, err
	}

	go func() {
		<-ctx.Done()
		evm.Cancel()
	}()

	gp := new(core.GasPool).AddGas(math.MaxUint64)
	res, gas, failed, err := core.ApplyMessage(evm, msg, gp)
	if err := vmError(); err != nil {
		return nil, 0, false, err
	}
	return res, gas, failed, err
}

func (s *PublicBlockChainAPI) Call(ctx context.Context, args CallArgs, blockNr rpc.BlockNumber) (hexutil.Bytes, error) {
	result, _, _, err := s.doCall(ctx, args, blockNr, vm.Config{}, 5*time.Second)
	return (hexutil.Bytes)(result), err
}

func (s *PublicBlockChainAPI) EstimateGas(ctx context.Context, args CallArgs) (hexutil.Uint64, error) {
	fmt.Printf("+++++++++++++++++++++++++++++++++++++++++++++estimate gas %v\n", args)

	var (
		lo  uint64 = params.TxGas - 1
		hi  uint64
		cap uint64
	)
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

	executable := func(gas uint64) bool {
		args.Gas = hexutil.Uint64(gas)

		_, _, failed, err := s.doCall(ctx, args, rpc.PendingBlockNumber, vm.Config{}, 0)
		if err != nil || failed {
			return false
		}
		return true
	}

	for lo+1 < hi {
		mid := (hi + lo) / 2
		if !executable(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}

	if hi == cap {
		if !executable(hi) {
			return 0, fmt.Errorf("gas required exceeds allowance or always failing transaction")
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
		"number":           (*hexutil.Big)(head.Number),
		"mainchainNumber":  (*hexutil.Big)(head.MainChainNumber),
		"hash":             b.Hash(),
		"parentHash":       head.ParentHash,
		"nonce":            head.Nonce,
		"mixHash":          head.MixDigest,
		"sha3Uncles":       head.UncleHash,
		"logsBloom":        head.Bloom,
		"stateRoot":        head.Root,
		"miner":            head.Coinbase.String(),
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
	BlockHash   common.Hash    `json:"blockHash"`
	BlockNumber *hexutil.Big   `json:"blockNumber"`
	From        string         `json:"from"`
	Gas         hexutil.Uint64 `json:"gas"`
	GasPrice    *hexutil.Big   `json:"gasPrice"`
	Hash        common.Hash    `json:"hash"`
	Input       hexutil.Bytes  `json:"input"`
	Nonce       hexutil.Uint64 `json:"nonce"`

	To               interface{}  `json:"to"`
	TransactionIndex hexutil.Uint `json:"transactionIndex"`
	Value            *hexutil.Big `json:"value"`
	V                *hexutil.Big `json:"v"`
	R                *hexutil.Big `json:"r"`
	S                *hexutil.Big `json:"s"`
}

func newRPCTransaction(tx *types.Transaction, blockHash common.Hash, blockNumber uint64, index uint64) *RPCTransaction {
	var signer types.Signer = types.FrontierSigner{}
	if tx.Protected() {
		signer = types.NewEIP155Signer(tx.ChainId())
	}
	from, _ := types.Sender(signer, tx)

	var to interface{}
	if tx.To() == nil {
		to = nil
	} else {
		to = tx.To().String()
	}

	v, r, s := tx.RawSignatureValues()
	result := &RPCTransaction{
		From:     from.String(),
		Gas:      hexutil.Uint64(tx.Gas()),
		GasPrice: (*hexutil.Big)(tx.GasPrice()),
		Hash:     tx.Hash(),
		Input:    hexutil.Bytes(tx.Data()),
		Nonce:    hexutil.Uint64(tx.Nonce()),
		To:       to,
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
	var to interface{}
	if tx.To() == nil {
		to = nil
	} else {
		to = tx.To().String()
	}

	fields := map[string]interface{}{
		"blockHash":         blockHash,
		"blockNumber":       hexutil.Uint64(blockNumber),
		"transactionHash":   hash,
		"transactionIndex":  hexutil.Uint64(index),
		"from":              from.String(),
		"to":                to,
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
	} else {
		var log []*Log
		for _, l := range receipt.Logs {
			newLog := &Log{
				Address:     l.Address.String(),
				Topics:      l.Topics,
				Data:        hexutil.Encode(l.Data),
				BlockNumber: l.BlockNumber,
				TxHash:      l.TxHash,
				TxIndex:     l.TxIndex,
				BlockHash:   l.BlockHash,
				Index:       l.Index,
				Removed:     l.Removed,
			}
			log = append(log, newLog)
		}
		fields["logs"] = log
	}

	if receipt.ContractAddress != (common.Address{}) {
		fields["contractAddress"] = receipt.ContractAddress.String()
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
	} else if !crypto.ValidateNEATAddr(string(args.To[:])) {
		return errors.New(`invalid address`)
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

		log.Info("Submitted contract creation", "fullhash", tx.Hash().Hex(), "contract", addr.String())
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

var (
	minimumRegisterAmount = math.MustParseBig256("77000000000000000000000")

	maxCandidateNumber = 1000

	maxDelegationAddresses = 1000

	maxEditValidatorLength = 1000
)

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

func (api *PublicNEATAPI) WithdrawReward(ctx context.Context, from common.Address, delegateAddress common.Address, gasPrice *hexutil.Big) (common.Hash, error) {
	input, err := neatAbi.ChainABI.Pack(neatAbi.WithdrawReward.String(), delegateAddress)
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.WithdrawReward.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.ChainContractMagicAddr,
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
		To:       &neatAbi.ChainContractMagicAddr,
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
		To:       &neatAbi.ChainContractMagicAddr,
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
		To:       &neatAbi.ChainContractMagicAddr,
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
		To:       &neatAbi.ChainContractMagicAddr,
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

func (api *PublicNEATAPI) GetBannedStatus(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[string]interface{}, error) {
	state, _, err := api.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}

	fields := map[string]interface{}{
		"banned":      state.GetBanned(address),
		"bannedEpoch": state.GetBannedTime(address),
		"blocks":      state.GetMinedBlocks(address),
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
		To:       &neatAbi.ChainContractMagicAddr,
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
		To:       &neatAbi.ChainContractMagicAddr,
		Gas:      (*hexutil.Uint64)(&defaultGas),
		GasPrice: gasPrice,
		Value:    nil,
		Input:    (*hexutil.Bytes)(&input),
		Nonce:    nil,
	}

	return SendTransaction(ctx, args, api.am, api.b, api.nonceLock)
}

func (api *PublicNEATAPI) UnBanned(ctx context.Context, from common.Address, gasPrice *hexutil.Big) (common.Hash, error) {
	input, err := neatAbi.ChainABI.Pack(neatAbi.UnBanned.String())
	if err != nil {
		return common.Hash{}, err
	}

	defaultGas := neatAbi.UnBanned.RequiredGas()

	args := SendTxArgs{
		From:     from,
		To:       &neatAbi.ChainContractMagicAddr,
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

	core.RegisterValidateCb(neatAbi.UnBanned, unBannedValidateCb)
	core.RegisterApplyCb(neatAbi.UnBanned, unBannedApplyCb)
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

	reward := state.GetRewardBalanceByDelegateAddress(from, args.DelegateAddress)
	state.SubRewardBalanceByDelegateAddress(from, args.DelegateAddress, reward)
	state.AddBalance(from, reward)

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
	fmt.Printf("register pubkey unmarshal json start\n")
	if verror != nil {
		return verror
	}
	fmt.Printf("register pubkey %v\n", blsPK)
	state.ApplyForCandidate(from, blsPK.KeyString(), args.Commission)

	state.MarkAddressCandidate(from)

	verror = updateNextEpochValidatorVoteSet(tx, state, bc, from, ops)
	if verror != nil {
		return verror
	}

	return nil
}

func registerValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.RegisterArgs, error) {
	candidateSet := state.GetCandidateSet()
	if len(candidateSet) > maxCandidateNumber {
		return nil, core.ErrMaxCandidate
	}

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

			state.AddPendingRefundBalanceByUser(from, key, depositProxiedBalance)

			state.MarkDelegateAddressRefund(from)
		}
		return true
	})

	state.CancelCandidate(from, allRefund)

	fmt.Printf("candidate set bug, unregiser clear candidate before\n")
	fmt.Printf("candidate set bug, unregiser clear candidate before %v\n", state.GetCandidateSet())

	state.ClearCandidateSetByAddress(from)
	fmt.Printf("candidate set bug, unregiser clear candidate after\n")
	fmt.Printf("candidate set bug, unregiser clear candidate after %v\n", state.GetCandidateSet())

	return nil
}

func unRegisterValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {

	if !state.IsCandidate(from) {
		return core.ErrNotCandidate
	}

	if state.GetBanned(from) {
		return core.ErrBannedUnRegister
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

	if !state.GetBanned(from) {
		verror = updateNextEpochValidatorVoteSet(tx, state, bc, args.Candidate, ops)
		if verror != nil {
			return verror
		}
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

	return nil
}

func unDelegateValidation(from common.Address, tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) (*neatAbi.UnDelegateArgs, error) {

	var args neatAbi.UnDelegateArgs
	data := tx.Data()
	if err := neatAbi.ChainABI.UnpackMethodInputs(&args, neatAbi.UnDelegate.String(), data[4:]); err != nil {
		return nil, err
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

func unBannedValidateCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain) error {
	from := derivedAddressFromTx(tx)

	err := unBannedValidation(from, state, bc)
	if err != nil {
		return err
	}

	return nil
}

func unBannedApplyCb(tx *types.Transaction, state *state.StateDB, bc *core.BlockChain, ops *types.PendingOps) error {
	from := derivedAddressFromTx(tx)
	err := unBannedValidation(from, state, bc)
	if err != nil {
		return err
	}

	state.SetBanned(from, false)

	state.ClearBannedSetByAddress(from)

	return nil
}

func unBannedValidation(from common.Address, state *state.StateDB, bc *core.BlockChain) error {
	if !state.IsCandidate(from) {
		return core.ErrNotCandidate
	}

	verror := updateValidation(bc)
	if verror != nil {
		return verror
	}

	if !state.GetBanned(from) {
		return fmt.Errorf("should not unbanned")
	}

	bannedEpoch := state.GetBannedTime(from)
	fmt.Printf("Unbannedden validation, banned epoch %v\n", bannedEpoch)

	if bannedEpoch.Cmp(common.Big0) == 1 {
		return fmt.Errorf("please unbanned %v epoch later", bannedEpoch)
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

	if currHeight == 1 || currHeight == ep.StartBlock || currHeight == ep.StartBlock+1 || currHeight == ep.EndBlock {
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

	fmt.Printf("update next epoch voteset %v\n", ep.GetEpochValidatorVoteSet())
	currentEpochVoteSet := ep.GetEpochValidatorVoteSet()
	fmt.Printf("update next epoch current epoch voteset %v\n", ep.GetEpochValidatorVoteSet())

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

		state.ForEachProxied(candidate, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {

			state.SubProxiedBalanceByUser(candidate, key, proxiedBalance)
			state.AddDepositProxiedBalanceByUser(candidate, key, proxiedBalance)
			return true
		})

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
