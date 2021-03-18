// Copyright 2015 The go-ethereum Authors
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

package params

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the Neatio main network.
var MainnetBootnodes = []string{

	"enode://e4b47ca874d94e44a401bbe57dc69505852cbeeadc812b7e92ef24ba74a011a171fd9800fdbc00a109035ee0129448b879f4a6cfcc96ec717597277997480b2d@127.0.0.1:9910",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Neatio test network.
var TestnetBootnodes = []string{
	"enode://e4b47ca874d94e44a401bbe57dc69505852cbeeadc812b7e92ef24ba74a011a171fd9800fdbc00a109035ee0129448b879f4a6cfcc96ec717597277997480b2d@127.0.0.1:9911",
}
