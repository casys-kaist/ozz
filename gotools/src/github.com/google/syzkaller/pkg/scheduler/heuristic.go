package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

type chunk interleaving.SerialAccess

// TODO: I think we can implement this using more of the previous
// implementation
func ComputePotentialBuggyKnots(seq []interleaving.SerialAccess) []interleaving.Segment {
	if len(seq) != 2 {
		return nil
	}

	cs0, cs1 := chunknize(seq[0]), chunknize(seq[1])
	// TODO: optimize
	knots := []interleaving.Segment{}
	for _, c0 := range cs0 {
		knots = append(knots, computePotentialBuggyKnots(c0, chunk(seq[1]))...)
	}
	for _, c1 := range cs1 {
		knots = append(knots, computePotentialBuggyKnots(c1, chunk(seq[0]))...)
	}
	// TODO: optimize
	ht := make(map[uint64]struct{})
	res := []interleaving.Segment{}
	for _, knot := range knots {
		hsh := knot.Hash()
		if _, ok := ht[hsh]; ok {
			continue
		}
		ht[hsh] = struct{}{}
		res = append(res, knot)
	}
	return res
}

func chunknize(serial interleaving.SerialAccess) []chunk {
	chunks := []chunk{}
	start := 0
	size := 0
	create := false
	for i, acc := range serial {
		if acc.Typ == interleaving.TypeFlush {
			size = i - start
			create = true
		} else if i == len(serial)-1 {
			size = len(serial) - start
			create = true
		}

		if create {
			if size > 1 {
				new := append(chunk{}, serial[start:i]...)
				if contain_store(new) {
					chunks = append(chunks, new)
				}
			}
			start = i + 1
			create = false
		}
	}
	return chunks
}

func contain_store(c chunk) bool {
	for _, acc := range c {
		if acc.Typ == interleaving.TypeStore {
			return true
		}
	}
	return false
}

func computePotentialBuggyKnots(c0, c1 chunk) []interleaving.Segment {
	knotter := GetKnotter(opts)
	knotter.AddSequentialTrace(
		[]interleaving.SerialAccess{
			interleaving.SerializeAccess(c0),
			interleaving.SerializeAccess(c1),
		})
	knotter.ExcavateKnots()
	return knotter.GetKnots()
}

var opts = KnotterOpts{
	Flags: FlagWantMessagePassing |
		FlagWantParallel |
		FlagDifferentAccessTypeOnly |
		FlagWantStrictMessagePassing,
}
