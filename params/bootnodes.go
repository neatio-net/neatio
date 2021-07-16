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
	"enode://9cd4e7bca9c220d39903c5b6a4ed1a70310cdaf975fdf38e75cd679e0e1669a3f3ebcdb088c1d6e4ac28f30f6386f4e4186bf4f3ac5a300d2f8046200a664cd9@135.181.195.79:9910",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Neatio test network.
var TestnetBootnodes = []string{
	"enode://b0d0db3876616f19f80417950ea8d43483611754bc564797d9048bf64953f0e20410fed5583b8aeca34778563d1480724b33427187afdb06344cfa8ec863c056@135.181.195.79:9910",
	"enode://b0d0db3876616f19f80417950ea8d43483611754bc564797d9048bf64953f0e20410fed5583b8aeca34778563d1480724b33427187afdb06344cfa8ec863c056@135.181.195.79:9911",
}
