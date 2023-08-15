package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

type chunk interleaving.SerialAccess

func ComputeHints(seq []interleaving.SerialAccess) []interleaving.Segment {
	if len(seq) != 2 {
		return nil
	}
	cs0, cs1 := chunknize(seq[0]), chunknize(seq[1])
	// TODO: optimize
	pso := computeHintsPSO(cs0, cs1, seq)
	tso := computeHintsTSO(cs0, cs1, seq)

	ht := make(map[uint64]struct{})
	res := []interleaving.Segment{}
	// TODO: Is this a bug? Why do we have dupped knots in {pso,tso}?
	dedup_append := func(knots []interleaving.Segment) {
		for _, knot := range knots {
			hsh := knot.Hash()
			if _, ok := ht[hsh]; ok {
				continue
			}
			ht[hsh] = struct{}{}
			res = append(res, knot)
		}
	}
	dedup_append(pso)
	dedup_append(tso)
	return res
}

func chunknize(serial interleaving.SerialAccess) []chunk {
	chunks := []chunk{}
	start := 0
	size := 0
	create, has_store := false, false
	for i, acc := range serial {
		if acc.Typ == interleaving.TypeStore {
			has_store = true
		} else if acc.Typ == interleaving.TypeFlush {
			size = i - start
			create = true
		} else if i == len(serial)-1 {
			size = len(serial) - start
			create = true
		}

		if create {
			if size > 1 && has_store {
				new := append(chunk{}, serial[start:i]...)
				chunks = append(chunks, new)
			}
			start = i + 1
			create, has_store = false, false
		}
	}
	return chunks
}

func computeHintsPSO(cs0, cs1 []chunk, seq []interleaving.SerialAccess) []interleaving.Segment {
	knots := []interleaving.Segment{}
	for _, c0 := range cs0 {
		knots = append(knots, __computeHints(c0, chunk(seq[1]), psoOpts)...)
	}
	for _, c1 := range cs1 {
		knots = append(knots, __computeHints(c1, chunk(seq[0]), psoOpts)...)
	}
	return knots
}

var psoOpts = KnotterOpts{
	Flags: FlagWantMessagePassing |
		FlagWantParallel |
		FlagDifferentAccessTypeOnly |
		FlagReassignThreadID |
		FlagWantStrictMessagePassing,
}

func computeHintsTSO(cs0, cs1 []chunk, seq []interleaving.SerialAccess) []interleaving.Segment {
	knots := []interleaving.Segment{}
	for _, c0 := range cs0 {
		for _, c1 := range cs1 {
			knots = append(knots, __computeHints(c0, c1, tsoOpts)...)
		}
	}
	return knots
}

func __computeHints(c0, c1 chunk, opts KnotterOpts) []interleaving.Segment {
	knotter := GetKnotter(opts)
	knotter.AddSequentialTrace(
		[]interleaving.SerialAccess{
			interleaving.SerializeAccess(c0),
			interleaving.SerializeAccess(c1),
		})
	knotter.ExcavateKnots()
	return knotter.GetKnots()
}

var tsoOpts = KnotterOpts{
	Flags: FlagWantParallel |
		FlagReassignThreadID |
		FlagDifferentAccessTypeOnly |
		FlagWantOOTA,
}
