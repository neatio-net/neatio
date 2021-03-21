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

	"enode://f2937477b350392fcb399beafe47fa7e23878eb852da8b234dfd2bc7e4dc5f4acd41ff25234d28b1ba25916268d54abd2a3a5ba7e741a7f86ba05102a465d6cd@localhost:9910",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Neatio test network.
var TestnetBootnodes = []string{
	"enode://f4caa1a33b8740093103b3866a42f51d55c65c314cfb81328a20968fc58f9c489d14a49f88a60a60d1193a3f4fdaeb1e51b3b1f83c51ed920e34853e05be9329@135.181.44.117:9911",
}
