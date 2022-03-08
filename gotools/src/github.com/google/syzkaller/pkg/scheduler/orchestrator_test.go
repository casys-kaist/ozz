package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func TestSelectHarmoniousKnotsIterSimple(t *testing.T) {
	for _, test := range testsSingleSeq {
		testSelectHarmoniousKnotsIter(t, test.filename, test.answer)
	}
}

func testSelectHarmoniousKnotsIter(t *testing.T, path string, answer primitive.Knot) {
	knots := loadKnots(t, []string{path})

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
