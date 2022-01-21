package scheduler

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

// helper function
func loadTestdata(t *testing.T, raw []byte) (threads [2]primitive.SerialAccess) {
	timestamp, thread := uint32(0), -1
	for {
		idx := bytes.IndexByte(raw, byte('\n'))
		if idx == -1 {
			break
		}
		line := raw[:idx]
		raw = raw[idx+1:]

		toks := bytes.Fields(line)
		if len(toks) != 3 {
			if bytes.HasPrefix(line, []byte("Thread")) {
				thread++
			}
			continue
		}

		var typ uint32
		if bytes.Equal(toks[2], []byte("R")) {
			typ = primitive.TypeLoad
		} else {
			typ = primitive.TypeStore
		}

		inst, err := strconv.ParseUint(string(toks[0]), 16, 64)
		if err != nil {
			t.Errorf("parsing error: %v", err)
		}
		addr, err := strconv.ParseUint(string(toks[1][2:]), 16, 64)
		if err != nil {
			t.Errorf("parsing error: %v", err)
		}

		acc := primitive.Access{
			Inst:      uint32(inst),
			Addr:      uint32(addr),
			Typ:       typ,
			Size:      4,
			Timestamp: timestamp,
			Thread:    uint64(thread),
		}
		threads[thread].Add(acc)
		timestamp++
	}
	return
}

func TestExcavateKnots(t *testing.T) {
	path := filepath.Join("testdata", "data1_simple")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	thrs := loadTestdata(t, data)
	knots := ExcavateKnots(thrs[:])

	t.Logf("# of knots: %d", len(knots))

	required := Knot{
		{{Inst: 0x8bbb79d6, Addr: 0x18a48520, Size: 4, Typ: primitive.TypeLoad, Timestamp: 6},
			{Inst: 0x8bbca80b, Addr: 0x18a48520, Size: 4, Typ: primitive.TypeStore, Timestamp: 156}},
		{{Inst: 0x8bbc9093, Addr: 0x18a48874, Size: 4, Typ: primitive.TypeLoad, Timestamp: 149},
			{Inst: 0x8bbb75a0, Addr: 0x18a48874, Size: 4, Typ: primitive.TypeStore, Timestamp: 14}},
	}

	found := false
	for i, knot := range knots {
		t.Logf("Knot %d, type: %v", i, knot.Type())
		t.Logf("  %x (%v) --> %x (%v)", knot[0][0].Inst, knot[0][0].Timestamp, knot[0][1].Inst, knot[0][1].Timestamp)
		t.Logf("  %x (%v) --> %x (%v)", knot[1][0].Inst, knot[1][0].Timestamp, knot[1][1].Inst, knot[1][1].Timestamp)
		if knot.Same(required) {
			t.Logf("found")
			found = true
		}
	}

	if !found {
		t.Errorf("can't find the required knot")
	}
}

func TestKnotType(t *testing.T) {
	tests := []struct {
		knot Knot
		ans  KnotType
	}{
		{
			[2]Communication{
				{{Timestamp: 0, Thread: 0}, {Timestamp: 2, Thread: 1}},
				{{Timestamp: 1, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			KnotParallel,
		},
		{
			[2]Communication{
				{{Timestamp: 2, Thread: 1}, {Timestamp: 0, Thread: 0}},
				{{Timestamp: 3, Thread: 1}, {Timestamp: 1, Thread: 0}},
			},
			KnotParallel,
		},
		{
			[2]Communication{
				{{Timestamp: 0, Thread: 0}, {Timestamp: 3, Thread: 1}},
				{{Timestamp: 1, Thread: 1}, {Timestamp: 2, Thread: 0}},
			},
			KnotOverlapped,
		},
		{
			[2]Communication{
				{{Timestamp: 3, Thread: 1}, {Timestamp: 0, Thread: 0}},
				{{Timestamp: 2, Thread: 0}, {Timestamp: 1, Thread: 1}},
			},
			KnotInvalid,
		},
		{
			[2]Communication{
				{{Timestamp: 0, Thread: 1}, {Timestamp: 2, Thread: 0}},
				{{Timestamp: 1, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			KnotOverlapped,
		},
		{
			[2]Communication{
				{{Timestamp: 0, Thread: 1}, {Timestamp: 1, Thread: 0}},
				{{Timestamp: 2, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			KnotSeparated,
		},
		{
			[2]Communication{
				{{Timestamp: 2, Thread: 0}, {Timestamp: 3, Thread: 1}},
				{{Timestamp: 0, Thread: 1}, {Timestamp: 1, Thread: 0}},
			},
			KnotSeparated,
		},
	}
	for _, test := range tests {
		if got := test.knot.Type(); test.ans != got {
			t.Errorf("wrong, expected %v, got %v", test.ans, got)
		}
	}
}
