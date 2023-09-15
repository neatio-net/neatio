package neatdb

type Iterator interface {
	Next() bool

	Error() error

	Key() []byte

	Value() []byte

	Release()
}

type Iteratee interface {
	NewIterator() Iterator

	NewIteratorWithPrefix(prefix []byte) Iterator
}
