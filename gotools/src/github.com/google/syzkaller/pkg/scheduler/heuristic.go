package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

type temp struct {
	inst, addr uint32
}

func ComputeHints0(seq []interleaving.SerialAccess) []interleaving.Hint {
	if len(seq) != 2 {
		return nil
	}
	// TODO: optimzie
	copySeq := func(s0, s1 interleaving.SerialAccess, first int) []interleaving.SerialAccess {
		ht := make(map[temp]struct{})
		serial0 := interleaving.SerialAccess{}
		for i, acc := range s0 {
			t := temp{inst: acc.Inst, addr: acc.Addr}
			if _, ok := ht[t]; ok {
				continue
			}
			ht[t] = struct{}{}
			acc.Timestamp = uint32(i)
			acc.Thread = uint64(first)
			serial0 = append(serial0, acc)
		}
		serial1 := interleaving.SerialAccess{}
		for i, acc := range s1 {
			acc.Timestamp = uint32(i + len(serial0))
			acc.Thread = uint64(1 - first)
			serial1 = append(serial1, acc)
		}
		return []interleaving.SerialAccess{serial0, serial1}
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

	hints := []interleaving.Hint{}
	knots := knotter.knots
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
		hint := aggregateHints(critComm, grouped)
		hints = append(hints, hint)
	}
	return hints
}

func aggregateHints(critComm interleaving.Communication, grouped []interleaving.Knot) interleaving.Hint {
	// TODO: What are good names for someInst*?
	someInst := []interleaving.Access{}
	someInst2 := []interleaving.Access{}
	for _, knot := range grouped {
		someComm := knot[0]
		someInst = append(someInst, someComm.Former())
		someInst2 = append(someInst2, someComm.Latter())
	}
	return interleaving.Hint{
		DelayingInst: someInst,
		SomeInst:     someInst2,
		CriticalComm: critComm,
	}
}
