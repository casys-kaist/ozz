package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

func ComputeHints0(seq []interleaving.SerialAccess) []interleaving.Hint {
	if len(seq) != 2 {
		return nil
	}
	h0 := computeHints(copySeq(seq[0], seq[1], 0))
	h1 := computeHints(copySeq(seq[1], seq[0], 1))
	return append(h0, h1...)
}

func ComputeHints(seq []interleaving.SerialAccess) []interleaving.Hint {
	if len(seq) != 2 {
		return nil
	}
	return computeHints(copySeq(seq[0], seq[1], 0))
}

func copySeq(s0, s1 interleaving.SerialAccess, first int) (seq []interleaving.SerialAccess) {
	// TODO: Optimize. copySeq() unnecessary overhead to copy serials
	seq = make([]interleaving.SerialAccess, 2)
	for i, acc := range s0 {
		acc.Timestamp = uint32(i)
		acc.Thread = uint64(first)
		seq[0] = append(seq[0], acc)
	}
	for i, acc := range s1 {
		acc.Timestamp = uint32(i + len(seq[0]))
		acc.Thread = uint64(1 - first)
		seq[1] = append(seq[1], acc)
	}
	return seq
}

func computeHints(seq []interleaving.SerialAccess) []interleaving.Hint {
	// NOTE: This function assumes that seq[0] was executed before
	// seq[1]
	if len(seq) != 2 {
		return nil
	}
	knotter := Knotter{}
	knotter.AddSequentialTrace(seq)
	knotter.ExcavateKnots()
	knots := knotter.knots
	testingStoreBarrier := knotter.testingStoreBarrier
	testingLoadBarrier := knotter.testingLoadBarrier

	hints := []interleaving.Hint{}
	for hsh, grouped := range knots {
		for _, knot := range grouped {
			if hsh != knot[1].Hash() {
				panic("wrong")
			}
		}
		if len(grouped) == 0 {
			continue
		}
		critComm := grouped[0][1]
		hints0 := aggregateHints(critComm, grouped, testingStoreBarrier, testingLoadBarrier)
		hints = append(hints, hints0...)
	}
	return hints
}

func aggregateHints(critComm interleaving.Communication, grouped []interleaving.Knot, testingStoreBarrier, testingLoadBarrier map[uint64]struct{}) []interleaving.Hint {
	hints := []interleaving.Hint{}
	for _, opt := range []struct {
		cond map[uint64]struct{}
		typ  interleaving.HintType
	}{
		{testingStoreBarrier, interleaving.TestingStoreBarrier},
		{testingLoadBarrier, interleaving.TestingLoadBarrier},
	} {
		hint, ok := aggregateHintWithConditions(critComm, grouped, opt.cond, opt.typ)
		if ok {
			hints = append(hints, hint)
		}
	}
	return hints
}

func aggregateHintWithConditions(critComm interleaving.Communication, grouped []interleaving.Knot, conds map[uint64]struct{}, typ interleaving.HintType) (hint interleaving.Hint, ok bool) {
	// TODO: Too many unnecessary memory operations (e.g., using a
	// map, ...)?
	type instsT map[uint32]interleaving.Access
	f := func(insts instsT, acc interleaving.Access) {
		if _, ok := insts[acc.Addr]; !ok {
			insts[acc.Addr] = acc
			return
		}
		acc0 := insts[acc.Addr]
		if acc0.Timestamp > acc.Timestamp {
			return
		}
		insts[acc.Addr] = acc
	}
	toSlice := func(insts instsT) []interleaving.Access {
		ret := make([]interleaving.Access, 0, len(insts))
		for _, acc := range insts {
			ret = append(ret, acc)
		}
		return ret
	}
	preceding := make(instsT)
	following := make(instsT)
	for _, knot := range grouped {
		if critComm.Hash() != knot[1].Hash() {
			panic("wrong")
		}
		if _, ok := conds[knot.Hash()]; ok {
			comm := knot[0]
			f(preceding, comm.Former())
			f(following, comm.Latter())
		}
	}
	if len(preceding) == 0 || len(following) == 0 {
		return
	}
	hint = interleaving.Hint{
		PrecedingInsts: toSlice(preceding),
		FollowingInsts: toSlice(following),
		CriticalComm:   critComm,
		Typ:            typ,
	}
	if !sanitizeHint(hint) {
		// if sanitizeHint() fails, hint contains nothing to reorder
		return
	}
	ok = true
	return
}

func sanitizeHint(hint interleaving.Hint) bool {
	var accs []interleaving.Access
	var want uint32
	switch hint.Typ {
	case interleaving.TestingStoreBarrier:
		accs, want = hint.PrecedingInsts, interleaving.TypeStore
	case interleaving.TestingLoadBarrier:
		accs, want = hint.FollowingInsts, interleaving.TypeLoad
	}
	for _, acc := range accs {
		if acc.Typ == want {
			return true
		}
	}
	return false
}
