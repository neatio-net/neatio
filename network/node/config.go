package node

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nio-net/nio/network/rpc"

	"github.com/nio-net/nio/chain/accounts"
	"github.com/nio-net/nio/chain/accounts/keystore"
	"github.com/nio-net/nio/chain/accounts/usbwallet"
	"github.com/nio-net/nio/chain/log"
	"github.com/nio-net/nio/network/p2p"
	"github.com/nio-net/nio/network/p2p/discover"
	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/crypto"
)

const (
	datadirPrivateKey      = "nodekey"
	datadirDefaultKeyStore = "keystore"
	datadirStaticNodes     = "static-nodes.json"
	datadirTrustedNodes    = "trusted-nodes.json"
	datadirNodeDatabase    = "nodes"
)

type Config struct {
	Name string `toml:"-"`

	ChainId string `toml:",omitempty"`

	UserIdent string `toml:",omitempty"`

	Version string `toml:"-"`

	GeneralDataDir string

	DataDir string

	P2P p2p.Config

	KeyStoreDir string `toml:",omitempty"`

	UseLightweightKDF bool `toml:",omitempty"`

	NoUSB bool `toml:",omitempty"`

	IPCPath string `toml:",omitempty"`

	HTTPHost string `toml:",omitempty"`

	HTTPPort int `toml:",omitempty"`

	HTTPCors []string `toml:",omitempty"`

	HTTPVirtualHosts []string `toml:",omitempty"`

	HTTPModules []string `toml:",omitempty"`

	HTTPTimeouts rpc.HTTPTimeouts

	WSHost string `toml:",omitempty"`

	WSPort int `toml:",omitempty"`

	WSOrigins []string `toml:",omitempty"`

	WSModules []string `toml:",omitempty"`

	WSExposeAll bool `toml:",omitempty"`

	Logger log.Logger `toml:",omitempty"`
}

func (c *Config) IPCEndpoint() string {

	if c.IPCPath == "" {
		return ""
	}

	if runtime.GOOS == "windows" {
		if strings.HasPrefix(c.IPCPath, `\\.\pipe\`) {
			return c.IPCPath
		}
		return `\\.\pipe\` + c.ChainId + `\` + c.IPCPath
	}

	if filepath.Base(c.IPCPath) == c.IPCPath {
		if c.DataDir == "" {
			return filepath.Join(os.TempDir(), c.IPCPath)
		}
		return filepath.Join(c.DataDir, c.IPCPath)
	}
	return c.IPCPath
}

func (c *Config) NodeDB() string {
	if c.GeneralDataDir == "" {
		return ""
	}
	return c.ResolvePath(datadirNodeDatabase)
}

func DefaultIPCEndpoint(clientIdentifier string) string {
	if clientIdentifier == "" {
		clientIdentifier = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if clientIdentifier == "" {
			panic("empty executable name")
		}
	}
	config := &Config{DataDir: DefaultDataDir(), IPCPath: clientIdentifier + ".ipc"}
	return config.IPCEndpoint()
}

func (c *Config) HTTPEndpoint() string {
	if c.HTTPHost == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}

func DefaultHTTPEndpoint() string {
	config := &Config{HTTPHost: DefaultHTTPHost, HTTPPort: DefaultHTTPPort}
	return config.HTTPEndpoint()
}

func (c *Config) WSEndpoint() string {
	if c.WSHost == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.WSHost, c.WSPort)
}

func DefaultWSEndpoint() string {
	config := &Config{WSHost: DefaultWSHost, WSPort: DefaultWSPort}
	return config.WSEndpoint()
}

func (c *Config) NodeName() string {
	name := c.name()

	if c.UserIdent != "" {
		name += "/" + c.UserIdent
	}
	if c.Version != "" {
		name += "/v" + c.Version
	}
	name += "/" + runtime.GOOS + "-" + runtime.GOARCH
	name += "/" + runtime.Version()
	return name
}

func (c *Config) name() string {
	if c.Name == "" {
		progname := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if progname == "" {
			panic("empty executable name, set Config.Name")
		}
		return progname
	}
	return c.Name
}

var isOldGethResource = map[string]bool{
	"chaindata":          true,
	"nodes":              true,
	"nodekey":            true,
	"static-nodes.json":  true,
	"trusted-nodes.json": true,
}

var isGeneralResource = map[string]bool{
	"nodes":              true,
	"nodekey":            true,
	"static-nodes.json":  true,
	"trusted-nodes.json": true,
}

func (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}

	if c.name() == "neatio" && isOldGethResource[path] {
		oldpath := ""
		if c.Name == "neatio" {
			oldpath = filepath.Join(c.DataDir, path)
		}
		if oldpath != "" && common.FileExist(oldpath) {

			return oldpath
		}
	}

	if isGeneralResource[path] {
		return filepath.Join(c.GeneralDataDir, path)
	}

	return filepath.Join(c.instanceDir(), path)
}

func (c *Config) instanceDir() string {
	if c.DataDir == "" {
		return ""
	}

	return filepath.Join(c.DataDir, c.name())
}

func (c *Config) NodeKey() *ecdsa.PrivateKey {

	if c.P2P.PrivateKey != nil {
		return c.P2P.PrivateKey
	}

	if c.GeneralDataDir == "" {
		key, err := crypto.GenerateKey()
		if err != nil {
			log.Crit(fmt.Sprintf("Failed to generate ephemeral node key: %v", err))
		}
		return key
	}

	keyfile := c.ResolvePath(datadirPrivateKey)
	if key, err := crypto.LoadECDSA(keyfile); err == nil {
		return key
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		log.Crit(fmt.Sprintf("Failed to generate node key: %v", err))
	}

	if err := os.MkdirAll(c.GeneralDataDir, 0700); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
		return key
	}
	keyfile = c.ResolvePath(datadirPrivateKey)

	if err := crypto.SaveECDSA(keyfile, key); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
	}
	return key
}

func (c *Config) StaticNodes() []*discover.Node {
	return c.parsePersistentNodes(c.ResolvePath(datadirStaticNodes))
}

func (c *Config) TrustedNodes() []*discover.Node {
	return c.parsePersistentNodes(c.ResolvePath(datadirTrustedNodes))
}

func (c *Config) parsePersistentNodes(path string) []*discover.Node {

	if c.DataDir == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}

	var nodelist []string
	if err := common.LoadJSON(path, &nodelist); err != nil {
		log.Error(fmt.Sprintf("Can't load node file %s: %v", path, err))
		return nil
	}

	var nodes []*discover.Node
	for _, url := range nodelist {
		if url == "" {
			continue
		}
		node, err := discover.ParseNode(url)
		if err != nil {
			log.Error(fmt.Sprintf("Node URL %s: %v\n", url, err))
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func (c *Config) AccountConfig() (int, int, string, error) {
	scryptN := keystore.StandardScryptN
	scryptP := keystore.StandardScryptP
	if c.UseLightweightKDF {
		scryptN = keystore.LightScryptN
		scryptP = keystore.LightScryptP
	}

	var (
		keydir string
		err    error
	)
	switch {
	case filepath.IsAbs(c.KeyStoreDir):
		keydir = c.KeyStoreDir
	case c.DataDir != "":
		if c.KeyStoreDir == "" {
			keydir = filepath.Join(c.DataDir, datadirDefaultKeyStore)
		} else {
			keydir, err = filepath.Abs(c.KeyStoreDir)
		}
	case c.KeyStoreDir != "":
		keydir, err = filepath.Abs(c.KeyStoreDir)
	}
	return scryptN, scryptP, keydir, err
}

func makeAccountManager(conf *Config) (*accounts.Manager, string, error) {
	scryptN, scryptP, keydir, err := conf.AccountConfig()
	var ephemeral string
	if keydir == "" {

		keydir, err = ioutil.TempDir("", "neatio-keystore")
		ephemeral = keydir
	}

	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(keydir, 0700); err != nil {
		return nil, "", err
	}

	backends := []accounts.Backend{
		keystore.NewKeyStore(keydir, scryptN, scryptP),
	}
	if !conf.NoUSB {

		if ledgerhub, err := usbwallet.NewLedgerHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Ledger hub, disabling: %v", err))
		} else {
			backends = append(backends, ledgerhub)
		}

		if trezorhub, err := usbwallet.NewTrezorHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Trezor hub, disabling: %v", err))
		} else {
			backends = append(backends, trezorhub)
		}
	}
	return accounts.NewManager(backends...), ephemeral, nil
}
