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
