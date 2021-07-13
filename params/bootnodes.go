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
	"enode://3f1bfc1c168616450bbce8ff3d228b53d7d142c1910afb7b58928e5011559d4b472fdb6f90331b1f81e823f5820b723f1d8ff3b7d93ff614ca3d53c1efd67aee@135.181.195.79:9910",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Neatio test network.
var TestnetBootnodes = []string{
	"enode://b0d0db3876616f19f80417950ea8d43483611754bc564797d9048bf64953f0e20410fed5583b8aeca34778563d1480724b33427187afdb06344cfa8ec863c056@135.181.195.79:9910",
	"enode://b0d0db3876616f19f80417950ea8d43483611754bc564797d9048bf64953f0e20410fed5583b8aeca34778563d1480724b33427187afdb06344cfa8ec863c056@135.181.195.79:9911",
}
