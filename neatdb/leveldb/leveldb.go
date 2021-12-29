package leveldb

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neatlab/neatio/chain/log"
	"github.com/neatlab/neatio/neatdb"
	"github.com/neatlab/neatio/utilities/metrics"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	degradationWarnInterval = time.Minute

	minCache = 16

	minHandles = 16

	metricsGatheringInterval = 3 * time.Second
)

type Database struct {
	fn string
	db *leveldb.DB

	compTimeMeter    metrics.Meter
	compReadMeter    metrics.Meter
	compWriteMeter   metrics.Meter
	writeDelayNMeter metrics.Meter
	writeDelayMeter  metrics.Meter
	diskReadMeter    metrics.Meter
	diskWriteMeter   metrics.Meter

	quitLock sync.Mutex
	quitChan chan chan error

	log log.Logger
}

func New(file string, cache int, handles int, namespace string) (*Database, error) {

	if cache < minCache {
		cache = minCache
	}
	if handles < minHandles {
		handles = minHandles
	}
	logger := log.New("database", file)

	db, err := leveldb.OpenFile(file, &opt.Options{
		OpenFilesCacheCapacity: handles,
		BlockCacheCapacity:     cache / 2 * opt.MiB,
		WriteBuffer:            cache / 4 * opt.MiB,
		Filter:                 filter.NewBloomFilter(10),
	})
	if _, corrupted := err.(*errors.ErrCorrupted); corrupted {
		db, err = leveldb.RecoverFile(file, nil)
	}
	if err != nil {
		return nil, err
	}

	ldb := &Database{
		fn:       file,
		db:       db,
		log:      logger,
		quitChan: make(chan chan error),
	}
	ldb.compTimeMeter = metrics.NewRegisteredMeter(namespace+"compact/time", nil)
	ldb.compReadMeter = metrics.NewRegisteredMeter(namespace+"compact/input", nil)
	ldb.compWriteMeter = metrics.NewRegisteredMeter(namespace+"compact/output", nil)
	ldb.diskReadMeter = metrics.NewRegisteredMeter(namespace+"disk/read", nil)
	ldb.diskWriteMeter = metrics.NewRegisteredMeter(namespace+"disk/write", nil)
	ldb.writeDelayMeter = metrics.NewRegisteredMeter(namespace+"compact/writedelay/duration", nil)
	ldb.writeDelayNMeter = metrics.NewRegisteredMeter(namespace+"compact/writedelay/counter", nil)

	go ldb.meter(metricsGatheringInterval)
	return ldb, nil
}

func (db *Database) Close() error {
	db.quitLock.Lock()
	defer db.quitLock.Unlock()

	if db.quitChan != nil {
		errc := make(chan error)
		db.quitChan <- errc
		if err := <-errc; err != nil {
			db.log.Error("Metrics collection failed", "err", err)
		}
		db.quitChan = nil
	}
	err := db.db.Close()
	if err == nil {
		db.log.Info("Database closed")
	} else {
		db.log.Error("Failed to close database", "err", err)
	}
	return err
}

func (db *Database) Has(key []byte) (bool, error) {
	return db.db.Has(key, nil)
}

func (db *Database) Get(key []byte) ([]byte, error) {
	dat, err := db.db.Get(key, nil)
	if err != nil {
		return nil, err
	}
	return dat, nil
}

func (db *Database) Put(key []byte, value []byte) error {
	return db.db.Put(key, value, nil)
}

func (db *Database) Delete(key []byte) error {
	return db.db.Delete(key, nil)
}

func (db *Database) NewBatch() neatdb.Batch {
	return &batch{
		db: db.db,
		b:  new(leveldb.Batch),
	}
}

func (db *Database) NewIterator() neatdb.Iterator {
	return db.NewIteratorWithPrefix(nil)
}

func (db *Database) NewIteratorWithPrefix(prefix []byte) neatdb.Iterator {
	return db.db.NewIterator(util.BytesPrefix(prefix), nil)
}

func (db *Database) Stat(property string) (string, error) {
	return db.db.GetProperty(property)
}

func (db *Database) Compact(start []byte, limit []byte) error {
	return db.db.CompactRange(util.Range{Start: start, Limit: limit})
}

func (db *Database) Path() string {
	return db.fn
}

func (db *Database) meter(refresh time.Duration) {

	compactions := make([][]float64, 2)
	for i := 0; i < 2; i++ {
		compactions[i] = make([]float64, 3)
	}

	var iostats [2]float64

	var (
		delaystats      [2]int64
		lastWritePaused time.Time
	)

	var (
		errc chan error
		merr error
	)

	for i := 1; errc == nil && merr == nil; i++ {

		stats, err := db.db.GetProperty("leveldb.stats")
		if err != nil {
			db.log.Error("Failed to read database stats", "err", err)
			merr = err
			continue
		}

		lines := strings.Split(stats, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) != "Compactions" {
			lines = lines[1:]
		}
		if len(lines) <= 3 {
			db.log.Error("Compaction leveldbTable not found")
			merr = errors.New("compaction leveldbTable not found")
			continue
		}
		lines = lines[3:]

		for j := 0; j < len(compactions[i%2]); j++ {
			compactions[i%2][j] = 0
		}
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) != 6 {
				break
			}
			for idx, counter := range parts[3:] {
				value, err := strconv.ParseFloat(strings.TrimSpace(counter), 64)
				if err != nil {
					db.log.Error("Compaction entry parsing failed", "err", err)
					merr = err
					continue
				}
				compactions[i%2][idx] += value
			}
		}

		if db.compTimeMeter != nil {
			db.compTimeMeter.Mark(int64((compactions[i%2][0] - compactions[(i-1)%2][0]) * 1000 * 1000 * 1000))
		}
		if db.compReadMeter != nil {
			db.compReadMeter.Mark(int64((compactions[i%2][1] - compactions[(i-1)%2][1]) * 1024 * 1024))
		}
		if db.compWriteMeter != nil {
			db.compWriteMeter.Mark(int64((compactions[i%2][2] - compactions[(i-1)%2][2]) * 1024 * 1024))
		}

		writedelay, err := db.db.GetProperty("leveldb.writedelay")
		if err != nil {
			db.log.Error("Failed to read database write delay statistic", "err", err)
			merr = err
			continue
		}
		var (
			delayN        int64
			delayDuration string
			duration      time.Duration
			paused        bool
		)
		if n, err := fmt.Sscanf(writedelay, "DelayN:%d Delay:%s Paused:%t", &delayN, &delayDuration, &paused); n != 3 || err != nil {
			db.log.Error("Write delay statistic not found")
			merr = err
			continue
		}
		duration, err = time.ParseDuration(delayDuration)
		if err != nil {
			db.log.Error("Failed to parse delay duration", "err", err)
			merr = err
			continue
		}
		if db.writeDelayNMeter != nil {
			db.writeDelayNMeter.Mark(delayN - delaystats[0])
		}
		if db.writeDelayMeter != nil {
			db.writeDelayMeter.Mark(duration.Nanoseconds() - delaystats[1])
		}

		if paused && delayN-delaystats[0] == 0 && duration.Nanoseconds()-delaystats[1] == 0 &&
			time.Now().After(lastWritePaused.Add(degradationWarnInterval)) {
			db.log.Warn("Database compacting, degraded performance")
			lastWritePaused = time.Now()
		}
		delaystats[0], delaystats[1] = delayN, duration.Nanoseconds()

		ioStats, err := db.db.GetProperty("leveldb.iostats")
		if err != nil {
			db.log.Error("Failed to read database iostats", "err", err)
			merr = err
			continue
		}
		var nRead, nWrite float64
		parts := strings.Split(ioStats, " ")
		if len(parts) < 2 {
			db.log.Error("Bad syntax of ioStats", "ioStats", ioStats)
			merr = fmt.Errorf("bad syntax of ioStats %s", ioStats)
			continue
		}
		if n, err := fmt.Sscanf(parts[0], "Read(MB):%f", &nRead); n != 1 || err != nil {
			db.log.Error("Bad syntax of read entry", "entry", parts[0])
			merr = err
			continue
		}
		if n, err := fmt.Sscanf(parts[1], "Write(MB):%f", &nWrite); n != 1 || err != nil {
			db.log.Error("Bad syntax of write entry", "entry", parts[1])
			merr = err
			continue
		}
		if db.diskReadMeter != nil {
			db.diskReadMeter.Mark(int64((nRead - iostats[0]) * 1024 * 1024))
		}
		if db.diskWriteMeter != nil {
			db.diskWriteMeter.Mark(int64((nWrite - iostats[1]) * 1024 * 1024))
		}
		iostats[0], iostats[1] = nRead, nWrite

		select {
		case errc = <-db.quitChan:

		case <-time.After(refresh):

		}
	}

	if errc == nil {
		errc = <-db.quitChan
	}
	errc <- merr
}

type batch struct {
	db   *leveldb.DB
	b    *leveldb.Batch
	size int
}

func (b *batch) Put(key, value []byte) error {
	b.b.Put(key, value)
	b.size += len(value)
	return nil
}

func (b *batch) Delete(key []byte) error {
	b.b.Delete(key)
	b.size++
	return nil
}

func (b *batch) ValueSize() int {
	return b.size
}

func (b *batch) Write() error {
	return b.db.Write(b.b, nil)
}

func (b *batch) Reset() {
	b.b.Reset()
	b.size = 0
}

func (b *batch) Replay(w neatdb.Writer) error {
	return b.b.Replay(&replayer{writer: w})
}

type replayer struct {
	writer  neatdb.Writer
	failure error
}

func (r *replayer) Put(key, value []byte) {

	if r.failure != nil {
		return
	}
	r.failure = r.writer.Put(key, value)
}

func (r *replayer) Delete(key []byte) {

	if r.failure != nil {
		return
	}
	r.failure = r.writer.Delete(key)
}
