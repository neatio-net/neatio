package metrics

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/neatio-network/neatio/chain/log"
)

var Enabled bool = false

const MetricsEnabledFlag = "metrics"

func init() {
	for _, arg := range os.Args {
		if flag := strings.TrimLeft(arg, "-"); flag == MetricsEnabledFlag {
			log.Info("Enabling metrics collection")
			Enabled = true
		}
	}
}

func CollectProcessMetrics(refresh time.Duration) {

	if !Enabled {
		return
	}

	memstats := make([]*runtime.MemStats, 2)
	diskstats := make([]*DiskStats, 2)
	for i := 0; i < len(memstats); i++ {
		memstats[i] = new(runtime.MemStats)
		diskstats[i] = new(DiskStats)
	}

	memAllocs := GetOrRegisterMeter("system/memory/allocs", DefaultRegistry)
	memFrees := GetOrRegisterMeter("system/memory/frees", DefaultRegistry)
	memInuse := GetOrRegisterMeter("system/memory/inuse", DefaultRegistry)
	memPauses := GetOrRegisterMeter("system/memory/pauses", DefaultRegistry)

	var diskReads, diskReadBytes, diskWrites, diskWriteBytes Meter
	if err := ReadDiskStats(diskstats[0]); err == nil {
		diskReads = GetOrRegisterMeter("system/disk/readcount", DefaultRegistry)
		diskReadBytes = GetOrRegisterMeter("system/disk/readdata", DefaultRegistry)
		diskWrites = GetOrRegisterMeter("system/disk/writecount", DefaultRegistry)
		diskWriteBytes = GetOrRegisterMeter("system/disk/writedata", DefaultRegistry)
	} else {
		log.Debug("Failed to read disk metrics", "err", err)
	}

	for i := 1; ; i++ {
		runtime.ReadMemStats(memstats[i%2])
		memAllocs.Mark(int64(memstats[i%2].Mallocs - memstats[(i-1)%2].Mallocs))
		memFrees.Mark(int64(memstats[i%2].Frees - memstats[(i-1)%2].Frees))
		memInuse.Mark(int64(memstats[i%2].Alloc - memstats[(i-1)%2].Alloc))
		memPauses.Mark(int64(memstats[i%2].PauseTotalNs - memstats[(i-1)%2].PauseTotalNs))

		if ReadDiskStats(diskstats[i%2]) == nil {
			diskReads.Mark(diskstats[i%2].ReadCount - diskstats[(i-1)%2].ReadCount)
			diskReadBytes.Mark(diskstats[i%2].ReadBytes - diskstats[(i-1)%2].ReadBytes)
			diskWrites.Mark(diskstats[i%2].WriteCount - diskstats[(i-1)%2].WriteCount)
			diskWriteBytes.Mark(diskstats[i%2].WriteBytes - diskstats[(i-1)%2].WriteBytes)
		}
		time.Sleep(refresh)
	}
}
