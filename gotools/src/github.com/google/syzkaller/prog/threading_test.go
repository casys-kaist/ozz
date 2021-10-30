package prog

import "testing"

func TestRazzerThreading(t *testing.T) {
	l := 7
	p := simpleRazzerProg(l)

	racing := RacingCalls{Calls: []int{2, 5}}

	p.Threading(racing)

	correct := [][2]uint64{
		// epoch, thred
		{0, 0},
		{1, 0},
		{4, 0},
		{2, 1},
		{3, 1},
		{4, 1},
		{5, 1},
	}
	for i := 0; i < l; i++ {
		if p.Calls[i].Epoch != correct[i][0] || p.Calls[i].Thread != correct[i][1] {
			t.Errorf("wrong: call=%d, expected: epoch=%d, thread=%d, got epoch=%d, thread=%d",
				i, correct[i][0], correct[i][1], p.Calls[i].Epoch, p.Calls[i].Thread)
		}
	}
}

func simpleRazzerProg(l int) *Prog {
	calls := []*Call{}
	for i := 0; i < l; i++ {
		call := &Call{Epoch: uint64(i), Thread: 0}
		calls = append(calls, call)
	}
	return &Prog{Calls: calls}
}
