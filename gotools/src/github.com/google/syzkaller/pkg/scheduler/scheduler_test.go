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

func testCVE(t *testing.T, knots []Knot, required Knot) bool {
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

func testCVE20168655(t *testing.T, knots []Knot) bool {
	required := Knot{
		{{Inst: 0x8bbb79d6, Addr: 0x18a48520, Size: 4, Typ: primitive.TypeLoad, Timestamp: 6},
			{Inst: 0x8bbca80b, Addr: 0x18a48520, Size: 4, Typ: primitive.TypeStore, Timestamp: 156}},
		{{Inst: 0x8bbc9093, Addr: 0x18a48874, Size: 4, Typ: primitive.TypeLoad, Timestamp: 149},
			{Inst: 0x8bbb75a0, Addr: 0x18a48874, Size: 4, Typ: primitive.TypeStore, Timestamp: 14}},
	}
	return testCVE(t, knots, required)
}

func testCVE20196974(t *testing.T, knots []Knot) bool {
	required := Knot{
		{{Inst: 0x81f2b4e1, Addr: 0x18a48520, Size: 4, Typ: primitive.TypeStore, Timestamp: 6}, // T0
			{Inst: 0x81f2bbd3, Addr: 0x18a48520, Size: 4, Typ: primitive.TypeLoad, Timestamp: 156}}, // T1
		{{Inst: 0x8d34b095, Addr: 0x18a48874, Size: 4, Typ: primitive.TypeStore, Timestamp: 149}, // T1
			{Inst: 0x8d3662f0, Addr: 0x18a48874, Size: 4, Typ: primitive.TypeLoad, Timestamp: 14}}, // T0
	}
	return testCVE(t, knots, required)
}

func testExcavateKnots(t *testing.T, filename string, testFunc func(*testing.T, []Knot) bool) []Knot {
	path := filepath.Join("testdata", filename)
	knots := loadKnots(t, path)
	if !testFunc(t, knots) {
		t.Errorf("can't find the required knot")
	}
	return knots
}

func TestExcavateKnots(t *testing.T) {
	testExcavateKnots(t, "data1", testCVE20168655)
	testExcavateKnots(t, "data2", testCVE20196974)
}

func TestExcavateKnotsSimple(t *testing.T) {
	knots := testExcavateKnots(t, "data1_simple", testCVE20168655)
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

func testSelectHarmoniousKnotsIter(t *testing.T, filename string, testFunc func(*testing.T, []Knot) bool) {
	path := filepath.Join("testdata", filename)
	knots := loadKnots(t, path)

	orch := orchestrator{knots: knots}
	i, count := 0, 0
	for len(orch.knots) != 0 {
		selected := orch.selectHarmoniousKnots()
		count += len(selected)
		t.Logf("Selected:")
		found := testFunc(t, selected)
		if found {
			t.Logf("Found: %d", i)
		}
		i++
	}

	if count != len(knots) {
		t.Errorf("wrong number of selected knots, expected %v, got %v", len(knots), count)
	}
}

func TestSelectHarmoniousKnotsIterSimple(t *testing.T) {
	testSelectHarmoniousKnotsIter(t, "data1_simple", testCVE20168655)
}

func TestSelectHarmoniousKnotsIter(t *testing.T) {
	testSelectHarmoniousKnotsIter(t, "data1", testCVE20168655)
	testSelectHarmoniousKnotsIter(t, "data2", testCVE20196974)
}

func TestGenerateSchedPoint(t *testing.T) {
	path := filepath.Join("testdata", "data1_simple")
	knots := loadKnots(t, path)

	orch := orchestrator{knots: knots}
	for len(orch.knots) != 0 {
		selected := orch.selectHarmoniousKnots()
		for i, knot := range selected {
			t.Logf("Knot %d, type: %v", i, knot.Type())
			t.Logf("  %x (%v) --> %x (%v)", knot[0][0].Inst, knot[0][0].Timestamp, knot[0][1].Inst, knot[0][1].Timestamp)
			t.Logf("  %x (%v) --> %x (%v)", knot[1][0].Inst, knot[1][0].Timestamp, knot[1][1].Inst, knot[1][1].Timestamp)
		}
		totalAcc := make(map[primitive.Access]struct{})
		for _, knot := range selected {
			for _, comm := range knot {
				totalAcc[comm.Former()] = struct{}{}
				totalAcc[comm.Latter()] = struct{}{}
			}
		}
		sched := Scheduler{knots: selected}
		sps := sched.GenerateSchedPoints()
		t.Logf("total %d sched points\n", len(sps))
		for _, sp := range sps {
			t.Logf("%v", primitive.Access(sp))
		}
		if len(totalAcc) != len(sps) {
			t.Errorf("missing schedpoint (before squeeze), expected %v, got %v", len(totalAcc), len(sps))
		}
		for _, knot := range selected {
			for _, comm := range knot {
				former, latter := false, false
				for _, sp := range sps {
					if primitive.Access(sp) == comm.Latter() {
						if !former {
							// we haven't seen the former one, so the
							// scheduling points are wrong
							t.Errorf("wrong schedpoint, the latter one is observed first, %v, %v",
								comm.Former(), comm.Latter())
						}
						latter = true
					}
					if primitive.Access(sp) == comm.Former() {
						// Former checking does not need another condition checking
						former = true
					}
				}
				if !former || !latter {
					t.Errorf("missing access, former found %v, latter found %v, former %v latter %v",
						former, latter, comm.Former(), comm.Latter())
				}
			}
		}
	}
}
