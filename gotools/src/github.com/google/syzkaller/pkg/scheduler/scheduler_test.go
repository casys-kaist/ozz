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

func loadKnots(t *testing.T, path string) []Knot {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	thrs := loadTestdata(t, data)
	knots := ExcavateKnots(thrs[:])
	t.Logf("# of knots: %d", len(knots))
	return knots
}

func testCVE20168655(t *testing.T, knots []Knot) bool {
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
	return found
}

func testExcavateKnots(t *testing.T, simple bool) []Knot {
	filename := "data1"
	if simple {
		filename = "data1_simple"
	}
	path := filepath.Join("testdata", filename)
	knots := loadKnots(t, path)
	if !testCVE20168655(t, knots) {
		t.Errorf("can't find the required knot")
	}
	return knots
}

func TestExcavateKnots(t *testing.T) {
	testExcavateKnots(t, false)
}

func TestExcavateKnotsSimple(t *testing.T) {
	knots := testExcavateKnots(t, true)
	totalKnots := 16
	if len(knots) != totalKnots {
		t.Errorf("wrong total number of knots, expected %v, got %v", totalKnots, len(knots))
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
		knot := test.knot
		if got := knot.Type(); test.ans != got {
			t.Errorf("wrong, expected %v, got %v", test.ans, got)
		}
	}
}

func testSelectHarmoniousKnotsIter(t *testing.T, simple bool) {
	filename := "data1"
	if simple {
		filename = "data1_simple"
	}
	path := filepath.Join("testdata", filename)
	knots := loadKnots(t, path)

	orch := orchestrator{knots: knots}
	count := 0
	for len(orch.knots) != 0 {
		selected := orch.selectHarmoniousKnots()
		count += len(selected)
		t.Logf("Selected:")
		testCVE20168655(t, selected)
	}

	if count != len(knots) {
		t.Errorf("wrong number of selected knots, expected %v, got %v", len(knots), count)
	}
}

func TestSelectHarmoniousKnotsIterSimple(t *testing.T) {
	testSelectHarmoniousKnotsIter(t, true)
}

func TestSelectHarmoniousKnotsIter(t *testing.T) {
	testSelectHarmoniousKnotsIter(t, false)
}
