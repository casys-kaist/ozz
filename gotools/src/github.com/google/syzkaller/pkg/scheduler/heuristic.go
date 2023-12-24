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
	// XXX: This function assumes that seq[0] was executed before
	// seq[1]
	if len(seq) != 2 {
		return nil
	}
	knotter := Knotter{}
	knotter.AddSequentialTrace(seq)
	knotter.ExcavateKnots()
	knots := knotter.GetKnots()
	_ = knots
	return nil
}
