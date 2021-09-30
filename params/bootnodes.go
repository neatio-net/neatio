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
// NEAT Blockchain main network.
var MainnetBootnodes = []string{

	"enode://8f4e613b97453ebcc21cd0df31c23bf82c92ca2afda1ecb54f0c63de64ddf0bbffbd1d7104c353fcbd50838e7d06fd2a56ed0338809a1eb8682d223228898c0a@135.181.195.79:9910",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// NEAT Blockchain test network.
var TestnetBootnodes = []string{
	"enode://eb0e6f3cd8f53cf36e82a6ff061cbd7fe31bd76b41bb4681bb4d11601ca3e7f913f69cf25d31861111e816f73f20ac6b44a39f2722c244a14dd805f36a6ee9f3@67.131.25.124:9911",
}
