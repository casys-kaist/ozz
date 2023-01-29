package main

import (
	"flag"
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
	if proc.scheduled < uint64(float64(proc.executed)*0.4) {
		proc.fuzzer.addCollection(CollectionUnplug, 1)
		proc.threadingPlugged = false
	}
}

func (proc *Proc) plugThreading() {
	if proc.scheduled > uint64(float64(proc.executed)*0.7) {
		proc.fuzzer.addCollection(CollectionPlug, 1)
		proc.threadingPlugged = true
	}
}
