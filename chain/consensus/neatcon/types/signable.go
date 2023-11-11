package types

import (
	"bytes"
	"io"

	. "github.com/neatio-net/common-go"
	"github.com/neatio-net/merkle-go"
)

type Signable interface {
	WriteSignBytes(chainID string, w io.Writer, n *int, err *error)
}

func SignBytes(chainID string, o Signable) []byte {
	buf, n, err := new(bytes.Buffer), new(int), new(error)
	o.WriteSignBytes(chainID, buf, n, err)
	if *err != nil {
		PanicCrisis(err)
	}
	return buf.Bytes()
}

func HashSignBytes(chainID string, o Signable) []byte {
	return merkle.SimpleHashFromBinary(SignBytes(chainID, o))
}
