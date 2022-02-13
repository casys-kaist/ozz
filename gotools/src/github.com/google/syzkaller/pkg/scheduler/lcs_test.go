package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func TestLCS(t *testing.T) {
	tests := []struct {
		serials []primitive.SerialAccess
		ans     [][]bool
	}{
		{
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 3}, {Inst: 7}, {Inst: 10}},
				{{Inst: 2}, {Inst: 3}, {Inst: 10}},
			},
			[][]bool{
				{false, true, false, true},
				{false, true, true},
			},
			// primitive.SerialAccess{{Inst: 3}, {Inst: 10}},
		},
		{
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 3}, {Inst: 7}, {Inst: 10}},
				{{Inst: 0}, {Inst: 3}, {Inst: 10}},
			},
			[][]bool{
				{true, true, false, true},
				{true, true, true},
			},
			// primitive.SerialAccess{{Inst: 0}, {Inst: 3}, {Inst: 10}},
		},
		{
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 1}},
				{{Inst: 2}, {Inst: 3}},
			},
			[][]bool{
				{false, false},
				{false, false},
			},
			// primitive.SerialAccess{},
		},
	}

	for _, test := range tests {
		length, bitmaps := lcs(test.serials[0], test.serials[1])
		t.Logf("len(LCS): %d", length)
		if len(test.ans) != len(bitmaps) {
			t.Errorf("wrong length, expected=%v, got=%v", len(test.ans), len(bitmaps))
		}
		for i := 0; i < len(test.ans); i++ {
			if len(test.ans[i]) != len(bitmaps[i]) {
				t.Errorf("wrong length, expected=%v, got=%v", len(test.ans[i]), len(bitmaps[i]))
			}
			for j := 0; j < len(test.ans[i]); j++ {
				if test.ans[i][j] != bitmaps[i][j] {
					t.Errorf("wrong at %d, expected=%v, got=%v", i, test.ans[i], bitmaps[i])
				}
			}
		}
	}
}
