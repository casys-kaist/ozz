package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestChunknize(t *testing.T) {
	seq := loadTestdata(t, []string{"pso_test"}, nil)[0]
	for i, serial := range seq {
		t.Logf("Chunknizing %d-th serial", i)
		chunks := chunkize(serial)
		t.Logf("# of chunks: %d", len(chunks))
		for i, chunk := range chunks {
			t.Logf("%d-th chunk", i)
			for _, acc := range chunk {
				t.Logf("%v", acc)
			}
		}
	}
}

func TestComputePotentialBuggyKnots(t *testing.T) {
	filename := "pso_test"
	ans := interleaving.Knot{
		{{Inst: 0x81a6167c, Size: 4, Typ: interleaving.TypeStore, Timestamp: 6}, {Inst: 0x81a61ba4, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1750}},
		{{Inst: 0x81a616a6, Size: 4, Typ: interleaving.TypeStore, Timestamp: 8}, {Inst: 0x81a61af7, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1749}}}

	seq := loadTestdata(t, []string{filename}, nil)[0]
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
	if !checkAnswer(t, knots, ans) {
		t.Errorf("%s: can't find the required knot", filename)
	}
}
