package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestNeedScheduling(t *testing.T) {
	proc := &Proc{
		fuzzer: &Fuzzer{
			workQueue: &WorkQueue{
				threading: make([]*WorkThreading, 100),
			},
		},
		rnd: rand.New(rand.NewSource(time.Now().UnixNano() + int64(0)*1e12)),
	}

	tests := [][2]uint64{
		{100, 0},
		{100, 20},
		{100, 40},
		{100, 50},
		{100, 60},
		{100, 80},
	}
	for _, test := range tests {
		proc.executed, proc.scheduled = test[0], test[1]
		cnt := 0
		for i := 0; i < 1000; i++ {
			if ok := proc.needScheduling(); ok {
				cnt++
			}
		}
		t.Logf("executed=%d scheduled=%d: %d", proc.executed, proc.scheduled, cnt)
	}
}
