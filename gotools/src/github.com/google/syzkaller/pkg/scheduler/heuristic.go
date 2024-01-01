package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

func ComputeHints0(seq []interleaving.SerialAccess) []interleaving.Hint {
	if len(seq) != 2 {
		return nil
	}
	// TODO: optimzie
	copySeq := func(s0, s1 interleaving.SerialAccess, first int) (seq []interleaving.SerialAccess) {
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
	h0 := ComputeHints(copySeq(seq[0], seq[1], 0))
	h1 := ComputeHints(copySeq(seq[1], seq[0], 1))
	return append(h0, h1...)
}

func ComputeHints(seq []interleaving.SerialAccess) []interleaving.Hint {
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
	storeHint := aggregateHintWithConditions(critComm, grouped, testingStoreBarrier, interleaving.TestingStoreBarrier)
	loadHint := aggregateHintWithConditions(critComm, grouped, testingLoadBarrier, interleaving.TestingLoadBarrier)
	hints = append(hints, storeHint)
	hints = append(hints, loadHint)
	return hints
}

func aggregateHintWithConditions(critComm interleaving.Communication, grouped []interleaving.Knot, conds map[uint64]struct{}, typ interleaving.HintType) interleaving.Hint {
	precedingInsts := []interleaving.Access{}
	followingInsts := []interleaving.Access{}
	for _, knot := range grouped {
		if critComm.Hash() != knot[1].Hash() {
			panic("wrong")
		}
		if _, ok := conds[knot.Hash()]; ok {
			comm := knot[0]
			precedingInsts = append(precedingInsts, comm.Former())
			followingInsts = append(followingInsts, comm.Latter())
		}
	}
	return interleaving.Hint{
		PrecedingInsts: precedingInsts,
		FollowingInsts: followingInsts,
		CriticalComm:   critComm,
		Typ:            typ,
	}
}
