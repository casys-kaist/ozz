package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestSelectHarmoniousKnotsIterSimple(t *testing.T) {
	for _, test := range testsSingleSeq {
		testSelectHarmoniousKnotsIter(t, test.filename, test.answer)
	}
}

func testSelectHarmoniousKnotsIter(t *testing.T, path string, answer interleaving.Knot) {
	knots := loadKnots(t, []string{path})

	segs := []interleaving.Segment{}
	for _, knot := range knots {
		segs = append(segs, knot)
	}

	orch := Orchestrator{Segs: segs}
	i, count := 0, 0
	for len(orch.Segs) != 0 {
		selected := orch.SelectHarmoniousKnots()
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
