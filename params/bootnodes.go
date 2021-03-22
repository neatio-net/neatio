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

	"enode://7eb31644b1287035867410f477eef6b069dee6226b682cb600 b38be977c9f26152901f73c53dbed86151286a17f126e17f0d371d656b98e6f878733c045b1f7a@135.181.154.74:9910",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Neatio test network.
var TestnetBootnodes = []string{
	"enode://f4caa1a33b8740093103b3866a42f51d55c65c314cfb81328a20968fc58f9c489d14a49f88a60a60d1193a3f4fdaeb1e51b3b1f83c51ed920e34853e05be9329@127.0.0.1:9911",
}
