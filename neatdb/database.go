package neatdb

import "io"

type Reader interface {
	Has(key []byte) (bool, error)

	Get(key []byte) ([]byte, error)
}

type Writer interface {
	Put(key []byte, value []byte) error

	Delete(key []byte) error
}

type Stater interface {
	Stat(property string) (string, error)
}

type Compacter interface {
	Compact(start []byte, limit []byte) error
}

type KeyValueStore interface {
	Reader
	Writer
	Batcher
	Iteratee
	Stater
	Compacter
	io.Closer
}

type Database interface {
	Reader
	Writer
	Batcher
	Iteratee
	Stater
	Compacter
	io.Closer
}
