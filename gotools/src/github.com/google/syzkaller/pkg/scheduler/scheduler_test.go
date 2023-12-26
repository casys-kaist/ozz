package scheduler

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestLockContending(t *testing.T) {
	tests := []struct {
		seq []interleaving.SerialAccess
		num int
	}{
		{
			seq: []interleaving.SerialAccess{
				{{0, 1, 0, interleaving.TypeLockAcquire, 0, 0}, {0x1, 0xabcd, 8, interleaving.TypeStore, 1, 0}, {0, 1, 0, interleaving.TypeLockRelease, 2, 0}},
				{{0, 1, 0, interleaving.TypeLockAcquire, 3, 1}, {0x1, 0xabcd, 8, interleaving.TypeLoad, 4, 1}, {0, 1, 0, interleaving.TypeLockRelease, 5, 1}},
			},
			num: 0,
		},
		{
			seq: []interleaving.SerialAccess{
				{{0, 1, 0, interleaving.TypeLockAcquire, 0, 0}, {0x1, 0xabcd, 8, interleaving.TypeStore, 1, 0}, {0, 1, 0, interleaving.TypeLockRelease, 2, 0}},
				{{0, 2, 0, interleaving.TypeLockAcquire, 3, 1}, {0x1, 0xabcd, 8, interleaving.TypeLoad, 4, 1}, {0, 2, 0, interleaving.TypeLockRelease, 5, 1}},
			},
			num: 1,
		},
	}
	for _, test := range tests {
		printSeq(t, test.seq)
		knotter := Knotter{}
		knotter.loopAllowed = loopAllowed
		knotter.AddSequentialTrace(test.seq)
		knotter.collectCommChans()
		knotter.buildAccessMap()
		knotter.annotateLocks()
		knotter.formCommunications()
		numComms := len(knotter.comms)
		for tid, locks := range knotter.locks {
			t.Logf("tid: %d, locks: %v", tid, locks)
		}
		if test.num != numComms {
			t.Errorf("Wrong, ans: %d, got: %d", test.num, numComms)
		}
	}
}

type singleInSameChunkTest struct {
	acc     [2]interleaving.Access
	store   bool
	allowed bool
}

func TestInSameChunk(t *testing.T) { //
	tests := []struct {
		fn   string
		test []singleInSameChunkTest
	}{
		{
			fn: "watchqueue",
			test: []singleInSameChunkTest{
				{acc: [2]interleaving.Access{
					{0x81c6f5fa, 0xb5cf020, 8, 0, 63084, 0},
					{0x81c6f69d, 0x7b92930, 4, 0, 63089, 0}},
					store:   true,
					allowed: true},
				{acc: [2]interleaving.Access{
					{0x81c6f56a, 0x253c357c, 4, 0, 63077, 0},
					{0x81c6f69d, 0x7b92930, 4, 0, 63089, 0}},
					store:   true,
					allowed: false},
				{acc: [2]interleaving.Access{
					{0x821fa4f2, 0x7b92930, 4, 1, 66942, 1},
					{0x821f94a8, 0x7b9293c, 4, 1, 66947, 1}},
					store:   false,
					allowed: true},
				{acc: [2]interleaving.Access{
					{0x821fa4f2, 0x7b92930, 4, 1, 66942, 1},
					{0x81703bc3, 0x7b92890, 4, 0, 67012, 1}},
					store:   false,
					allowed: false},
			},
		},
	}
	for _, test := range tests {
		seq := loadTestdata(t, test.fn)
		printSeq(t, seq)
		knotter := Knotter{}
		knotter.loopAllowed = loopAllowed
		knotter.AddSequentialTrace(seq)
		knotter.collectCommChans()
		knotter.buildAccessMap()
		knotter.annotateLocks()
		knotter.chunknizeSerials()
		knotter.formCommunications()
		for _, test0 := range test.test {
			if res := knotter.inSameChunk(test0.acc[0], test0.acc[1], test0.store); res != test0.allowed {
				t.Errorf("Wrong, want: %v, got: %v", test0.allowed, res)
			}
		}
	}
}

func TestFastenKnots(t *testing.T) {
	tests := []struct {
		fn   string
		hash uint64
		load bool
	}{
		{
			fn: "watchqueue",
			// scheduler_test.go:125: 146, a8759bd41e12b3d4
			// scheduler_test.go:126: thread #0: 81c6f61c accesses fb5cf010 (size: 8, type: 0, timestamp: 63085) -> thread #1: 821f962f accesses fb5cf010 (size: 8, type: 0, timestamp: 66988)
			// scheduler_test.go:127: thread #0: 81c6f69d accesses f7b92930 (size: 4, type: 0, timestamp: 63089) -> thread #1: 821f9408 accesses f7b92930 (size: 4, type: 1, timestamp: 66945)
			hash: 0xa8759bd41e12b3d4,
		},
		{
			fn:   "watchqueue2",
			hash: 0xa8759bd41e12b3d4,
			load: true,
		},
	}
	for _, test := range tests {
		seq := loadTestdata(t, test.fn)
		knotter := Knotter{}
		knotter.AddSequentialTrace(seq)
		knotter.ExcavateKnots()
		found := false
	loop:
		for _, knots := range knotter.knots {
			for _, knot := range knots {
				if knot.Hash() == test.hash {
					found = true
					break loop
				}
			}
		}
		if !found {
			t.Errorf("Failed to find a desired knot")
		}
		if test.load {
			if _, ok := knotter.testingLoadBarrier[test.hash]; !ok {
				t.Errorf("Wanted to test load reordering")
			}
		}
	}
}

func TestRedundantKnots(t *testing.T) {
	tests := []string{
		"watchqueue",
		"watchqueue2",
	}
	for _, test := range tests {
		seq := loadTestdata(t, test)
		knotter := Knotter{}
		knotter.AddSequentialTrace(seq)
		knotter.ExcavateKnots()
		ht := make(map[uint64]struct{})
		for _, grouped := range knotter.knots {
			for _, knot := range grouped {
				hsh := knot.Hash()
				if _, ok := ht[hsh]; ok {
					t.Errorf("Redundant hash found: %v", hsh)
				}
				ht[hsh] = struct{}{}
			}
		}
	}
}

func loadTestdata(t *testing.T, fn string) []interleaving.SerialAccess {
	path := filepath.Join("testdata", fn)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	res := make([]interleaving.SerialAccess, 2)
	var tid uint32
	for {
		idx := bytes.IndexByte(data, byte('\n'))
		if idx == -1 {
			break
		}
		line := string(data[:idx])
		data = data[idx+1:]

		toks := strings.Fields(line)
		if strings.HasPrefix(line, "serial") {
			// new
			tid0, err := strconv.Atoi(toks[1])
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			tid = uint32(tid0)
			continue
		}
		inst, _ := strconv.ParseUint(toks[0], 0, 64)
		addr, _ := strconv.ParseUint(toks[1], 0, 64)
		size, _ := strconv.ParseUint(toks[2], 10, 64)
		typ, _ := strconv.ParseUint(toks[3], 10, 64)
		ts, _ := strconv.ParseUint(toks[4], 10, 64)
		acc := interleaving.Access{
			Inst:      uint32(inst),
			Addr:      uint32(addr),
			Size:      uint32(size),
			Typ:       uint32(typ),
			Timestamp: uint32(ts),
			Thread:    uint64(tid),
		}
		res[tid] = append(res[tid], acc)
	}
	return res
}

func printSeq(t *testing.T, seq []interleaving.SerialAccess) {
	for i, serial := range seq {
		t.Logf("%d-serial", i)
		for _, acc := range serial {
			t.Logf("%v", acc)
		}
	}
}
