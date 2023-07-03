package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"runtime"

	"github.com/google/syzkaller/pkg/log"
)

var flagMonitor = flag.Bool("monitor-memory-usage", false, "moniro memory usage")

func MonitorMemUsage() {
	// ReadMemStats is very heavy, so unless we want, do not monitor
	// memory usage
	if !*flagMonitor {
		return
	}
	bToMb := func(b uint64) uint64 {
		return b / 1024 / 1024
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	log.Logf(0, "Alloc = %v MiB", bToMb(m.Alloc))
	log.Logf(0, "\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	log.Logf(0, "\tSys = %v MiB", bToMb(m.Sys))
	log.Logf(0, "\tNumGC = %v\n", m.NumGC)
}

func (proc *Proc) relieveMemoryPressure() {
	needSchedule := proc.fuzzer.spillScheduling()
	needThreading := proc.fuzzer.spillThreading()
	if !needSchedule && !needThreading {
		return
	}
	MonitorMemUsage()
	for cnt := 0; (needSchedule || needThreading) && cnt < 10; cnt++ {
		log.Logf(2, "Relieving memory pressure schedule=%v threading=%v", needSchedule, needThreading)
		if needSchedule {
			fuzzerSnapshot := proc.fuzzer.snapshot()
			proc.scheduleInput(fuzzerSnapshot)
		} else if item := proc.fuzzer.workQueue.dequeueThreading(); item != nil {
			proc.threadingInput(item)
		}
		needSchedule = proc.fuzzer.spillScheduling()
		needThreading = proc.fuzzer.spillThreading()
		if !needSchedule && !needThreading {
			break
		}
	}
	return
}

func (fuzzer *Fuzzer) spillCollection(collection Collection, threshold uint64) bool {
	fuzzer.corpusMu.RLock()
	defer fuzzer.corpusMu.RUnlock()
	return fuzzer.collection[collection] > threshold
}

const spillThreshold = uint64(100000)

func (fuzzer *Fuzzer) spillThreading() bool {
	return fuzzer.spillCollection(CollectionThreadingHint, spillThreshold)
}

func (fuzzer *Fuzzer) spillScheduling() bool {
	return fuzzer.spillCollection(CollectionScheduleHint, spillThreshold)
}

func (proc *Proc) powerSchedule() {
	if proc.threadingPlugged {
		proc.unplugThreading()
	} else {
		proc.plugThreading()
	}
}

func (proc *Proc) unplugThreading() {
	if proc.balancer.scheduled < uint64(float64(proc.balancer.executed)*0.4) {
		proc.fuzzer.addCollection(CollectionUnplug, 1)
		proc.threadingPlugged = false
	}
}

func (proc *Proc) plugThreading() {
	if proc.balancer.scheduled > uint64(float64(proc.balancer.executed)*0.7) {
		proc.fuzzer.addCollection(CollectionPlug, 1)
		proc.threadingPlugged = true
	}
}

type balancer struct {
	executed  uint64
	scheduled uint64
	// Values last printed
	executed0  uint64
	scheduled0 uint64
}

func (bal balancer) String() string {
	return fmt.Sprintf("executed=%v scheduled=%v", bal.executed, bal.scheduled)
}

func (bal *balancer) print() {
	if bal.executed0 != bal.executed || bal.scheduled0 != bal.scheduled {
		// Values has been chagned
		log.Logf(2, "%v", bal)
		bal.executed0, bal.scheduled0 = bal.executed, bal.scheduled
	}
}

func (bal *balancer) count(stat Stat) {
	bal.executed++
	if stat == StatSchedule || stat == StatThreading {
		bal.scheduled++
	}
}

func (bal balancer) needScheduling(r *rand.Rand) bool {
	// prob = 1 / (1 + exp(-25 * (-x + 0.25))) where x = (scheduled/executed)
	x := float64(bal.scheduled) / float64(bal.executed)
	prob1000 := int(1 / (1 + math.Exp(-30*(-1*x+0.25))) * 1000)
	if prob1000 < 50 {
		prob1000 = 50
	}
	return prob1000 >= r.Intn(1000)
}
