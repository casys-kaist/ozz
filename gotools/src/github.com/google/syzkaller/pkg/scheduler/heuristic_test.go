package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func testChunknize(t *testing.T, filename string, ans chunk) {
	check := func(c, ansc chunk, ok bool) bool {
		if ok {
			return true
		}
		i := 0
		for _, acc := range c {
			if acc.Inst == ansc[i].Inst {
				i++
				if i == len(ansc) {
					return true
				}
			}
		}
		return false
	}
	seq := loadTestdata(t, []string{filename}, nil)[0]
	var ok bool
	for i, serial := range seq {
		chunks := chunknize(serial)
		t.Logf("Chunknized %d-th serial (#: %d)", i, len(chunks))
		for j, chunk := range chunks {
			t.Logf("%d-th chunk", j)
			for _, acc := range chunk {
				t.Logf("%v", acc)
			}
			if check(chunk, ans, ok) {
				ok = true
			}
		}
	}
	if !ok {
		t.Errorf("%v: Failed to find a chunk", filename)
	}
}

func TestChunknize(t *testing.T) {
	tests := []struct {
		filename string
		ans      chunk
	}{
		{
			filename: "pso_test",
			ans: chunk{
				{Inst: 0x81a6167c}, {Inst: 0x81a616a6},
			},
		},
		{
			filename: "watchqueue_pipe",
			ans: chunk{
				{Inst: 0x81ad9a0c}, {Inst: 0x81ad9a84},
			},
		},
	}
	for _, test := range tests {
		testChunknize(t, test.filename, test.ans)
	}
}

func TestComputePotentialBuggyKnots(t *testing.T) {
	tests := []struct {
		filename string
		ans      interleaving.Knot
	}{
		{
			filename: "pso_test",
			ans: interleaving.Knot{
				{{Inst: 0x81a6167c, Size: 4, Typ: interleaving.TypeStore, Timestamp: 6}, {Inst: 0x81a61ba4, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1750}},
				{{Inst: 0x81a616a6, Size: 4, Typ: interleaving.TypeStore, Timestamp: 8}, {Inst: 0x81a61af7, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1749}}},
		},
		{
			filename: "watchqueue_pipe",
			ans: interleaving.Knot{
				{{Inst: 0x81ad9a0c, Size: 8, Typ: interleaving.TypeStore, Timestamp: 98}, {Inst: 0x81f83178, Size: 8, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 197}},
				{{Inst: 0x81ad9a84, Size: 4, Typ: interleaving.TypeStore, Timestamp: 102}, {Inst: 0x81f82be8, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 191}}},
		},
	}

	for _, test := range tests {
		seq := loadTestdata(t, []string{test.filename}, nil)[0]
		res := ComputePotentialBuggyKnots(seq[:])
		knots := []interleaving.Knot{}
		for _, knot0 := range res {
			knots = append(knots, knot0.(interleaving.Knot))
		}

		for i, knot0 := range res {
			knot := knot0.(interleaving.Knot)
			t.Logf("%d-th knot (%v)", i, knot.Hash())
			for _, comm := range knot {
				t.Logf("%v --> %v", comm.Former(), comm.Latter())
			}
		}
		if !checkAnswer(t, knots, test.ans) {
			t.Errorf("%s: can't find the required knot", test.filename)
		}
	}
}
