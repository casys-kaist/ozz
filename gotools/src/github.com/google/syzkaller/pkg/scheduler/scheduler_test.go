package scheduler

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

// TODO: answers depend on the data, so it should reside in the data
// file
var CVE20168655 = primitive.Knot{
	{{Inst: 0x8bbb79d6, Size: 4, Typ: primitive.TypeLoad}, {Inst: 0x8bbca80b, Size: 4, Typ: primitive.TypeStore}},
	{{Inst: 0x8bbc9093, Size: 4, Typ: primitive.TypeLoad}, {Inst: 0x8bbb75a0, Size: 4, Typ: primitive.TypeStore}}}

var CVE20196974 = primitive.Knot{
	{{Inst: 0x81f2b4e1, Size: 4, Typ: primitive.TypeStore}, {Inst: 0x81f2bbd3, Size: 4, Typ: primitive.TypeLoad}},
	{{Inst: 0x8d34b095, Size: 4, Typ: primitive.TypeStore}, {Inst: 0x8d3662f0, Size: 4, Typ: primitive.TypeLoad}}}

var tests = []struct {
	filename string
	answer   primitive.Knot
	total    int
}{
	{"data1", CVE20168655, -1},
	{"data2", CVE20196974, -1},
	{"data1_simple", CVE20168655, 16},
}

func TestSanitizeSequentialTrace(t *testing.T) {
	tests := []struct {
		seqs [][]primitive.SerialAccess
		ok   bool
	}{
		{[][]primitive.SerialAccess{
			[]primitive.SerialAccess{{{Thread: 0}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
		}, true}, // one sequential execution, two threads
		{[][]primitive.SerialAccess{
			[]primitive.SerialAccess{{{Thread: 0}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
			[]primitive.SerialAccess{{{Thread: 0}, {Thread: 0}, {Thread: 0}}, {{Thread: 1}}},
		}, true}, // two sequential execution, two threads
		{[][]primitive.SerialAccess{
			[]primitive.SerialAccess{{{Thread: 0}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
			[]primitive.SerialAccess{{{Thread: 0}, {Thread: 0}, {Thread: 0}}, {{Thread: 1}}, {{Thread: 2}}},
		}, false}, // two sequential execution, one has two threads, the other has three threads
		{[][]primitive.SerialAccess{
			[]primitive.SerialAccess{{{Thread: 0}, {Thread: 1}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
		}, false}, // one serial is not a single thread
		{[][]primitive.SerialAccess{
			[]primitive.SerialAccess{{{Thread: 0}}, {}},
		}, false}, // one serial is empty
		{[][]primitive.SerialAccess{
			[]primitive.SerialAccess{},
		}, false}, // empty seq
	}
	for i, test := range tests {
		knotter := Knotter{}
		var res bool
		for _, seq := range test.seqs {
			res = knotter.sanitizeSequentialTrace(seq)
			if !res {
				break
			}
		}
		if res != test.ok {
			t.Errorf("#%d wrong, expected=%v, got=%v", i, test.ok, res)
		}
	}
}

func TestCollectCommChans(t *testing.T) {
	test := struct {
		seqs  [][]primitive.SerialAccess
		after [][]primitive.SerialAccess
	}{
		[][]primitive.SerialAccess{
			{ // seq 0
				{{Addr: 1, Timestamp: 1, Typ: primitive.TypeStore}, {Addr: 13, Timestamp: 2, Typ: primitive.TypeLoad}},
				{{Addr: 1, Timestamp: 3, Typ: primitive.TypeLoad, Thread: 1}, {Addr: 102, Timestamp: 4, Typ: primitive.TypeStore, Thread: 1}},
			},
			{ // seq 1
				{{Addr: 204, Timestamp: 101, Typ: primitive.TypeLoad, Thread: 1}, {Addr: 305, Timestamp: 102, Typ: primitive.TypeLoad, Thread: 1}},
				{{Addr: 102, Timestamp: 103, Typ: primitive.TypeLoad}, {Addr: 305, Timestamp: 104, Typ: primitive.TypeStore}},
			},
		},
		[][]primitive.SerialAccess{
			{ // seq 0
				{{Addr: 1, Timestamp: 1, Typ: primitive.TypeStore}},
				{{Addr: 1, Timestamp: 3, Typ: primitive.TypeLoad, Thread: 1}, {Addr: 102, Timestamp: 4, Typ: primitive.TypeStore, Thread: 1}},
			},
			{ // seq 1
				{{Addr: 305, Timestamp: 102, Typ: primitive.TypeLoad, Thread: 1}},
				{{Addr: 102, Timestamp: 103, Typ: primitive.TypeLoad}, {Addr: 305, Timestamp: 104, Typ: primitive.TypeStore}},
			},
		},
	}
	knotter := Knotter{}
	for _, seq := range test.seqs {
		if !knotter.AddSequentialTrace(seq) {
			t.Fatalf("test case is wrong")
		}
	}
	knotter.collectCommChans()
	if !reflect.DeepEqual(test.after, knotter.seqs) {
		t.Errorf("wrong\nexptected=%v\ngot=%v", test.after, knotter.seqs)
	}
}

func TestInferProgramOrderThread(t *testing.T) {
	test := struct {
		serials []*primitive.SerialAccess
		commLen int
		bitmap  [][]bool
		ans     []primitive.SerialAccess
	}{
		[]*primitive.SerialAccess{
			&primitive.SerialAccess{{Timestamp: 100}, {Inst: 1, Timestamp: 101}, {Inst: 2, Timestamp: 102}, {Inst: 4, Timestamp: 103}},
			&primitive.SerialAccess{{Timestamp: 0}, {Inst: 3, Timestamp: 1}, {Inst: 4, Timestamp: 3}},
		},
		2,
		[][]bool{
			{true, false, false, true},
			{true, false, true},
		},
		[]primitive.SerialAccess{
			{{Timestamp: 0, Thread: commonPath << 16}, {Inst: 1, Timestamp: 1}, {Inst: 2, Timestamp: 2}, {Timestamp: 3, Thread: commonPath << 16}},
			{{Timestamp: 0, Thread: commonPath << 16}, {Inst: 3, Timestamp: 1, Thread: 1 << 16}, {Timestamp: 3, Thread: commonPath << 16}},
		},
	}

	knotter := Knotter{}
	knotter.inferProgramOrderThread(test.serials, test.commLen, test.bitmap)

	for i := range test.serials {
		serial := test.serials[i]
		for j := 1; j < len(*serial); j++ {
			if (*serial)[j].Timestamp <= (*serial)[j-1].Timestamp {
				t.Errorf("PO violation")
			}
			if ctx := ((*serial)[j].Thread >> 16) & 0xff; ctx == commonPath && (test.ans[i][j].Thread>>16)&0xff != ctx {
				t.Errorf("Common path wrong at %d %d, expected=%v, got=%v", i, j, test.ans[i][j].Thread, ctx)
			}
		}
	}
}

func TestExcavateKnots(t *testing.T) {
	for _, test := range tests {
		knots := testExcavateKnots(t, test.filename, test.answer)
		if test.total != -1 && len(knots) != test.total {
			t.Errorf("wrong total number of knots, expected %v, got %v", test.total, len(knots))
		}
	}
}

func testExcavateKnots(t *testing.T, filename string, answer primitive.Knot) []primitive.Knot {
	path := filepath.Join("testdata", filename)
	knots := loadKnots(t, path)
	if !checkAnswer(t, knots, answer) {
		t.Errorf("can't find the required knot")
	}
	return knots
}

func TestSelectHarmoniousKnotsIterSimple(t *testing.T) {
	for _, test := range tests {
		testSelectHarmoniousKnotsIter(t, test.filename, test.answer)
	}
}

func testSelectHarmoniousKnotsIter(t *testing.T, filename string, answer primitive.Knot) {
	path := filepath.Join("testdata", filename)
	knots := loadKnots(t, path)

	orch := orchestrator{knots: knots}
	i, count := 0, 0
	for len(orch.knots) != 0 {
		selected := orch.selectHarmoniousKnots()
		count += len(selected)
		t.Logf("Selected:")
		found := checkAnswer(t, selected, answer)
		if found {
			t.Logf("Found: %d", i)
		}
		i++
	}

	if count != len(knots) {
		t.Errorf("wrong number of selected knots, expected %v, got %v", len(knots), count)
	}
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
		sps, ok := sched.GenerateSchedPoints()
		t.Logf("total %d sched points\n", len(sps))
		for _, sp := range sps {
			t.Logf("%v", primitive.Access(sp))
		}
		if !ok {
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

func TestSqueezeSchedPoints(t *testing.T) {
	path := filepath.Join("testdata", "data1_simple")
	knots := loadKnots(t, path)
	orch := orchestrator{knots: knots}
	for len(orch.knots) != 0 {
		selected := orch.selectHarmoniousKnots()
		sched := Scheduler{knots: selected}
		full, ok := sched.GenerateSchedPoints()
		if !ok {
			t.Errorf("failed to generate a full schedule")
		}
		t.Logf("total %d full sched points\n", len(full))
		for _, sp := range full {
			t.Logf("%v", primitive.Access(sp))
		}
		squeezed := sched.SqueezeSchedPoints()
		t.Logf("total %d squeezed sched points\n", len(squeezed))
		for _, sp := range squeezed {
			t.Logf("%v", primitive.Access(sp))
		}
		j := 0
		for i := 0; i < len(full); i++ {
			if j < len(squeezed) && full[i] == squeezed[j] {
				j++
			}
		}
		if j != len(squeezed) {
			t.Errorf("squeezed sched is not a subset of full sched")
		}
		// TODO: check the squeezed sched points are correct.
	}
}

func BenchmarkExcavateKnots(b *testing.B) {
	benchmarkExcavateKnots(b)
}
