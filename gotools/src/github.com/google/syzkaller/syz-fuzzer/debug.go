package main

import (
	"sync"
	"time"

	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/log"
)

func printSeq(seq []interleaving.SerialAccess) {
	for i, serial := range seq {
		log.Logf(0, "%d-th", i)
		for _, acc := range serial {
			log.Logf(0, "%v", acc)
		}
	}
}

const (
	idle = iota
	triage
	candidate
	smash
	threading
	gen
	fuzz
	schedule
	total
	count
)

type monitor struct {
	ts    time.Time
	typ   int
	rec   [count]time.Duration
	total time.Duration
	sync.RWMutex
}

func (m *monitor) start() {
	if !_debug {
		return
	}
	m.Lock()
	defer m.Unlock()
	m.ts = time.Now()
	// To make end() panic if mark is not executed
	m.typ = total
}

func (m *monitor) end() {
	if !_debug || m.typ == idle {
		return
	}
	m.Lock()
	defer m.Unlock()
	e := time.Since(m.ts)
	if !(0 < m.typ && m.typ < total) {
		panic("debug wrong")
	}
	m.rec[m.typ] += e
	m.rec[total] += e
	m.typ = idle
}

func (m *monitor) mark(t int) {
	if !_debug {
		return
	}
	m.Lock()
	defer m.Unlock()
	m.typ = t
}

func (m *monitor) get() map[Collection]uint64 {
	m.Lock()
	defer m.Unlock()
	res := make(map[Collection]uint64)
	for i := 1; i < count; i++ {
		res[CollectionDurationTriage+Collection(i)-1] = uint64(m.rec[i].Nanoseconds())
	}
	return res
}

var _debug bool = true
