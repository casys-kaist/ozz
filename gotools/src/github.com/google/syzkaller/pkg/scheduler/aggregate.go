package scheduler

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
)

func GenerateCandidates(rnd *rand.Rand, hints []interleaving.Segment) (interleaving.Candidate, []interleaving.Segment, []interleaving.Segment) {
	idx := rnd.Intn(len(hints))
	pivot := hints[idx].(interleaving.Knot)
	if isMessagePassing(pivot[0], pivot[1]) {
		return aggregateCandidates(pivot[1], hints)
	} else {
		return aggregateCandidates(pivot[1], hints)
	}
}

func aggregateCandidates(critComm interleaving.Communication, hints []interleaving.Segment) (interleaving.Candidate, []interleaving.Segment, []interleaving.Segment) {
	collected, remaining := collectSegments(critComm, hints)
	cand := interleaving.Candidate{}
	return cand, collected, remaining
}

func collectSegments(critComm interleaving.Communication, hints []interleaving.Segment) (collected []interleaving.Segment, remaining []interleaving.Segment) {
	critHash := critComm.Hash()
	for _, seg := range hints {
		knot := seg.(interleaving.Knot)
		crits := extractCriticalCommunication(knot)
		for _, crit := range crits {
			if hsh := crit.Hash(); hsh == critHash {
				collected = append(collected, seg)
			} else {
				remaining = append(remaining, seg)
			}
		}
	}
	return
}

func extractCriticalCommunication(knot interleaving.Knot) []interleaving.Communication {
	if isMessagePassing(knot[0], knot[1]) {
		return []interleaving.Communication{knot[1]}
	} else {
		return []interleaving.Communication{knot[0], knot[1]}
	}
}

func constructCandidate(segs []interleaving.Segment) interleaving.Candidate {
	for _, seg := range segs {
		_, ok := seg.(interleaving.Knot)
		if !ok {
			panic("wrong")
		}
	}
	return interleaving.Candidate{}
}
