package scheduler

import (
	"path/filepath"
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