package neatdb

const IdealBatchSize = 100 * 1024

type Batch interface {
	Writer

	ValueSize() int

	Write() error

	Reset()

	Replay(w Writer) error
}

type Batcher interface {
	NewBatch() Batch
}
