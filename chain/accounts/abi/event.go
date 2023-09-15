package abi

import (
	"fmt"
	"strings"

	"github.com/nio-net/nio/utilities/common"
	"github.com/nio-net/nio/utilities/crypto"
)

type Event struct {
	Name string

	RawName   string
	Anonymous bool
	Inputs    Arguments
}

func (e Event) String() string {
	inputs := make([]string, len(e.Inputs))
	for i, input := range e.Inputs {
		inputs[i] = fmt.Sprintf("%v %v", input.Type, input.Name)
		if input.Indexed {
			inputs[i] = fmt.Sprintf("%v indexed %v", input.Type, input.Name)
		}
	}
	return fmt.Sprintf("event %v(%v)", e.RawName, strings.Join(inputs, ", "))
}

func (e Event) Sig() string {
	types := make([]string, len(e.Inputs))
	for i, input := range e.Inputs {
		types[i] = input.Type.String()
	}
	return fmt.Sprintf("%v(%v)", e.RawName, strings.Join(types, ","))
}

func (e Event) ID() common.Hash {
	return common.BytesToHash(crypto.Keccak256([]byte(e.Sig())))
}
