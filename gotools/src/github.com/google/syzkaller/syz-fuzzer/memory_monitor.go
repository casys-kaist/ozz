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
