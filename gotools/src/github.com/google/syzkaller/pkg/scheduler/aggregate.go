package scheduler

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
)

func GenerateCandidates(rnd *rand.Rand, hints []interleaving.Segment) (interleaving.Candidate, []interleaving.Segment, []interleaving.Segment) {
	idx := rnd.Intn(len(hints))
	pivot := hints[idx].(interleaving.Knot)
	critComm := extractCriticalCommunication(pivot)
	return aggregateCandidates(critComm, hints)
}

func aggregateCandidates(critComm interleaving.Communication, hints []interleaving.Segment) (interleaving.Candidate, []interleaving.Segment, []interleaving.Segment) {
	collected, remaining := collectSegments(critComm, hints)
	cand := constructCandidate(critComm, collected)
	return cand, collected, remaining
}

func collectSegments(critComm interleaving.Communication, hints []interleaving.Segment) (collected []interleaving.Segment, remaining []interleaving.Segment) {
	critHash := critComm.Hash()
	for _, seg := range hints {
		knot := seg.(interleaving.Knot)
		crit := extractCriticalCommunication(knot)
		if hsh := crit.Hash(); hsh == critHash {
			collected = append(collected, seg)
		} else {
			remaining = append(remaining, seg)
		}
	}
	return
}

func extractCriticalCommunication(knot interleaving.Knot) interleaving.Communication {
	// NOTE: knot[1] is always a criticall communication
	return knot[1]
}

func constructCandidate(critComm interleaving.Communication, segs []interleaving.Segment) interleaving.Candidate {
	// TODO: The reason why I name variables some* is that we don't
	// sufficiently understand characteristics/nature of such bugs,
	// someComm, and someInst. When we understand that, rename thus
	// variables.
	someInst := []interleaving.Access{}
	for _, seg := range segs {
		knot, ok := seg.(interleaving.Knot)
		if !ok {
			panic("wrong")
		}
		someComm := knot[0]
		someInst = append(someInst, someComm.Former())
	}
	return interleaving.Candidate{
		DelayingInst: someInst,
		CriticalComm: critComm,
	}
}
