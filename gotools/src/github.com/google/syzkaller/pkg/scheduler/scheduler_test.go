package scheduler

import (
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestSanitizeSequentialTrace(t *testing.T) {
	tests := []struct {
		seqs [][]interleaving.SerialAccess
		ok   bool
	}{
		{[][]interleaving.SerialAccess{
			{{{Thread: 0}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
		}, true}, // one sequential execution, two threads
		{[][]interleaving.SerialAccess{
			{{{Thread: 0}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
			{{{Thread: 0}, {Thread: 0}, {Thread: 0}}, {{Thread: 1}}},
		}, true}, // two sequential execution, two threads
		{[][]interleaving.SerialAccess{
			{{{Thread: 0}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
			{{{Thread: 0}, {Thread: 0}, {Thread: 0}}, {{Thread: 1}}, {{Thread: 2}}},
		}, false}, // two sequential execution, one has two threads, the other has three threads
		{[][]interleaving.SerialAccess{
			{{{Thread: 0}, {Thread: 1}, {Thread: 0}}, {{Thread: 1}, {Thread: 1}}},
		}, false}, // one serial is not a single thread
		{[][]interleaving.SerialAccess{
			{{{Thread: 0}}, {}},
		}, false}, // one serial is empty
		{[][]interleaving.SerialAccess{
			{},
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
		seqs  [][]interleaving.SerialAccess
		after [][]interleaving.SerialAccess
	}{
		[][]interleaving.SerialAccess{
			{ // seq 0
				{{Addr: 1, Timestamp: 1, Typ: interleaving.TypeStore}, {Inst: 1, Addr: 13, Timestamp: 2, Typ: interleaving.TypeLoad}},
				{{Inst: 3, Addr: 1, Timestamp: 3, Typ: interleaving.TypeLoad, Thread: 1}, {Inst: 4, Addr: 102, Timestamp: 4, Typ: interleaving.TypeStore, Thread: 1}},
			},
			{ // seq 1
				{{Addr: 204, Timestamp: 101, Typ: interleaving.TypeLoad, Thread: 1}, {Inst: 1, Addr: 305, Timestamp: 102, Typ: interleaving.TypeLoad, Thread: 1}},
				{{Inst: 3, Addr: 102, Timestamp: 103, Typ: interleaving.TypeLoad}, {Inst: 4, Addr: 305, Timestamp: 104, Typ: interleaving.TypeStore}},
			},
		},
		[][]interleaving.SerialAccess{
			{ // seq 0
				{{Addr: 1, Timestamp: 1, Typ: interleaving.TypeStore}},
				{{Inst: 3, Addr: 1, Timestamp: 3, Typ: interleaving.TypeLoad, Thread: 1}, {Inst: 4, Addr: 102, Timestamp: 4, Typ: interleaving.TypeStore, Thread: 1}},
			},
			{ // seq 1
				{{Inst: 1, Addr: 305, Timestamp: 102, Typ: interleaving.TypeLoad, Thread: 1}},
				{{Inst: 3, Addr: 102, Timestamp: 103, Typ: interleaving.TypeLoad}, {Inst: 4, Addr: 305, Timestamp: 104, Typ: interleaving.TypeStore}},
			},
		},
	}
	knotter := Knotter{loopAllowed: loopAllowed}
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

var testsSingleSeq = []struct {
	filename string
	answer   interleaving.Knot
	total    int
}{
	{"data1", CVE20168655, -1},
	{"data2", CVE20196974, -1},
	{"data1_simple", CVE20168655, -1},
}

func TestExcavateKnots(t *testing.T) {
	for _, test := range testsSingleSeq {
		knots := testExcavateKnots(t, test.filename, test.answer)
		if test.total != -1 && len(knots) != test.total {
			t.Errorf("wrong total number of knots, expected %v, got %v", test.total, len(knots))
		}
	}
}

func testExcavateKnots(t *testing.T, filename string, answer interleaving.Knot) []interleaving.Knot {
	knots := loadKnots(t, []string{filename}, KnotterOpts{})
	if !checkAnswer(t, knots, answer) {
		t.Errorf("%s: can't find the required knot", filename)
	}
	return knots
}

func TestGenerateSchedPoint(t *testing.T) {
	knots := loadKnots(t, []string{"data1_simple"}, KnotterOpts{})
	segs := []interleaving.Segment{}
	for _, knot := range knots {
		segs = append(segs, knot)
	}
	orch := Orchestrator{Segs: segs}
	for len(orch.Segs) != 0 {
		selected := orch.SelectHarmoniousKnots()
		for i, knot := range selected {
			t.Logf("Knot %d, type: %v", i, knot.Type())
			t.Logf("  %x (%v) --> %x (%v)", knot[0][0].Inst, knot[0][0].Timestamp, knot[0][1].Inst, knot[0][1].Timestamp)
			t.Logf("  %x (%v) --> %x (%v)", knot[1][0].Inst, knot[1][0].Timestamp, knot[1][1].Inst, knot[1][1].Timestamp)
		}
		totalAcc := make(map[interleaving.Access]struct{})
		for _, knot := range selected {
			for _, comm := range knot {
				totalAcc[comm.Former()] = struct{}{}
				totalAcc[comm.Latter()] = struct{}{}
			}
		}
		sched := Scheduler{Knots: selected}
		sps, ok := sched.GenerateSchedPoints()
		t.Logf("total %d sched points\n", len(sps))
		for _, sp := range sps {
			t.Logf("%v", interleaving.Access(sp))
		}
		if !ok {
			t.Errorf("missing schedpoint (before squeeze), expected %v, got %v", len(totalAcc), len(sps))
		}
		for _, knot := range selected {
			for _, comm := range knot {
				former, latter := false, false
				for _, sp := range sps {
					if interleaving.Access(sp) == comm.Latter() {
						if !former {
							// we haven't seen the former one, so the
							// scheduling points are wrong
							t.Errorf("wrong schedpoint, the latter one is observed first, %v, %v",
								comm.Former(), comm.Latter())
						}
						latter = true
					}
					if interleaving.Access(sp) == comm.Former() {
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

func TestExcavateKnotsTwoSeqs(t *testing.T) {
	tests := []struct {
		filenames []string
		answer    interleaving.Knot
	}{
		{
			[]string{"data1_seq1", "data1_seq2"},
			CVE20168655,
		},
		{
			[]string{"cve-2019-6974-seq1", "cve-2019-6974-seq2"},
			CVE20196974_2,
		},
		{
			[]string{"cve-2019-6974-2-seq1", "cve-2019-6974-2-seq2"},
			CVE20196974_3,
		},
	}
	for _, test := range tests {
		knots := loadKnots(t, test.filenames, KnotterOpts{})
		if !checkAnswer(t, knots, test.answer) {
			t.Errorf("%v: can't find the required knot", test.filenames)
		}
	}
}

func TestExcavateKnotsSingleThread(t *testing.T) {
	for _, test := range testsSingleSeq {
		thrs0 := loadTestdata(t, []string{test.filename}, nil)
		thrs := thrs0[0]
		for i := range thrs {
			for j := range thrs[i] {
				thrs[i][j].Thread = 0
			}
		}
		knotter := Knotter{
			opts: KnotterOpts{
				Flags: FlagReassignThreadID,
			},
		}
		knotter.AddSequentialTrace(thrs[:])
		knotter.ExcavateKnots()
		knots0 := knotter.GetKnots()
		knots := []interleaving.Knot{}
		for _, knot0 := range knots0 {
			knots = append(knots, knot0.(interleaving.Knot))
		}
		if !checkAnswer(t, knots, test.answer) {
			t.Errorf("can't find the required knot")
		}
	}
}

func TestDupCheck(t *testing.T) {
	knotter := &Knotter{}
	loadTestdata(t, []string{"cve-2019-6974-seq1", "cve-2019-6974-seq2"}, knotter)
	knotter.ExcavateKnots()

	comms := knotter.GetCommunications()
	t.Logf("Total communications: %d", len(comms))
	dupCommCnt := 0
	for i := range comms {
		for j := i + 1; j < len(comms); j++ {
			if comms[i].Hash() == comms[j].Hash() {
				t.Errorf("duplicated communication:\n%v\n%v", comms[i], comms[j])
				dupCommCnt++
			}
		}
	}
	t.Logf("Duplicated communications pairs: %d", dupCommCnt)
}

// TODO: answers depend on the data (i.e., two test data from the same
// CVE may differ depending on the binary they ran on such as
// CVE20196974 and CVE20196974_2), so it should reside in the data
// file.
var CVE20168655 = interleaving.Knot{
	{{Inst: 0x8bbb79d6, Size: 4, Typ: interleaving.TypeLoad, Timestamp: 6}, {Inst: 0x8bbca80b, Size: 4, Typ: interleaving.TypeStore, Thread: 1, Timestamp: 156}},
	{{Inst: 0x8bbc9093, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 149}, {Inst: 0x8bbb75a0, Size: 4, Typ: interleaving.TypeStore, Timestamp: 14}}}

var CVE20196974 = interleaving.Knot{
	{{Inst: 0x81f2b4e1, Size: 4, Typ: interleaving.TypeStore, Timestamp: 812}, {Inst: 0x81f2bbd3, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 845}},
	{{Inst: 0x8d34b095, Size: 4, Typ: interleaving.TypeStore, Thread: 1, Timestamp: 895}, {Inst: 0x8d3662f0, Size: 4, Typ: interleaving.TypeLoad, Timestamp: 824}}}

var CVE20196974_2 = interleaving.Knot{
	{{Inst: 0x8d57633a, Size: 4, Typ: interleaving.TypeStore, Thread: 1, Timestamp: 174}, {Inst: 0x8d592198, Size: 4, Typ: interleaving.TypeLoad, Timestamp: 336}},
	{{Inst: 0x81f9e606, Size: 4, Typ: interleaving.TypeStore, Timestamp: 331}, {Inst: 0x81f9ecf8, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 147}}}

var CVE20196974_3 = interleaving.Knot{
	{{Inst: 0x8d576637, Size: 4, Typ: interleaving.TypeStore, Thread: 1, Timestamp: 68}, {Inst: 0x8d592495, Size: 4, Typ: interleaving.TypeLoad, Timestamp: 276}},
	{{Inst: 0x81f9ebf6, Size: 4, Typ: interleaving.TypeStore, Timestamp: 230}, {Inst: 0x81f9f2e8, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 161}}}

func BenchmarkExcavateKnots(b *testing.B) {
	benchmarkExcavateKnots(b)
}
