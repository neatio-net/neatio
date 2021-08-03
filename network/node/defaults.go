// Copyright 2016 The go-ethereum Authors
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

package node

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/neatlab/neatio/network/p2p"
	"github.com/neatlab/neatio/network/p2p/nat"
	"github.com/neatlab/neatio/network/rpc"
)

const (
	DefaultHTTPHost = "localhost" // Default host interface for the HTTP RPC server
	DefaultHTTPPort = 9915        // Default TCP port for the HTTP RPC server
	DefaultWSHost   = "localhost" // Default host interface for the websocket RPC server
	DefaultWSPort   = 9916        // Default TCP port for the websocket RPC server
)

// DefaultConfig contains reasonable default settings.
var DefaultConfig = Config{
	GeneralDataDir:   DefaultDataDir(),
	DataDir:          DefaultDataDir(),
	HTTPPort:         DefaultHTTPPort,
	HTTPModules:      []string{"net", "web3"},
	HTTPVirtualHosts: []string{"localhost"},
	HTTPTimeouts:     rpc.DefaultHTTPTimeouts,
	WSPort:           DefaultWSPort,
	WSModules:        []string{"net", "web3"},
	P2P: p2p.Config{
		ListenAddr: ":9910",
		MaxPeers:   200,
		NAT:        nat.Any(),
	},
}

// DefaultDataDir is the default data directory to use for the databases and other
// persistence requirements.
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		//if runtime.GOOS == "darwin" {
		//	return filepath.Join(home, "Library", "neatio")
		//} else
		if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "neatio")
		} else {
			return filepath.Join(home, ".neatio")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
