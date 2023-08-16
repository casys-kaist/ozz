package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

type chunk interleaving.SerialAccess

// TODO: Not sure the current implementation is what we
// want. Currently, ComputeHints assumes that two given serials are
// executed in order and finds out hints with strict timestamps
// assumed. Although this is fine for delaying stores, but we may need
// to change it if we want to prefetching loads.

func ComputeHints0(seq []interleaving.SerialAccess) []interleaving.Segment {
	if len(seq) != 2 {
		return nil
	}
	// TODO: optimzie
	copySeq := func(s0, s1 interleaving.SerialAccess, first int) []interleaving.SerialAccess {
		var start0, start1 int
		if first == 0 {
			start0, start1 = 0, len(s0)
		} else {
			start0, start1 = len(s1), 0
		}
		serial0 := interleaving.SerialAccess{}
		for i, acc := range s0 {
			acc.Timestamp = uint32(i + start0)
			serial0 = append(serial0, acc)
		}
		serial1 := interleaving.SerialAccess{}
		for i, acc := range s1 {
			acc.Timestamp = uint32(i + start1)
			serial1 = append(serial1, acc)
		}
		return []interleaving.SerialAccess{serial0, serial1}
	}
	h0 := ComputeHints(copySeq(seq[0], seq[1], 0))
	h1 := ComputeHints(copySeq(seq[0], seq[1], 1))
	return append(h0, h1...)
}

func ComputeHints(seq []interleaving.SerialAccess) []interleaving.Segment {
	// XXX: This function assumes that seq[0] was executed before
	// seq[1]
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
	return knots
}

var psoOpts = KnotterOpts{
	Flags: FlagWantMessagePassing,
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
	commonFlags := (FlagStrictTimestamp |
		FlagWantParallel |
		FlagDifferentAccessTypeOnly |
		FlagReassignThreadID |
		FlagMultiVariableOnly)
	opts.Flags |= commonFlags
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
	Flags: FlagWantOOTA,
}
