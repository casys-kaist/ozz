package scheduler

import "github.com/google/syzkaller/pkg/primitive"

type Orchestrator struct {
	// Communications that are already selected
	comms []primitive.Communication
	// Input knots
	Segs []primitive.Segment
}

// TODO: The time complexity of orchestrator.SelectHarmoniousKnots()
// is O(n*n). Reduce it to O(n).

func (orch *Orchestrator) SelectHarmoniousKnots() []primitive.Knot {
	res := []primitive.Knot{}
	remaining := make([]primitive.Segment, 0, len(orch.Segs))
	cnt := 0
	for _, seg := range orch.Segs {
		if knot, ok := seg.(primitive.Knot); ok && orch.harmoniousKnot(knot) {
			res = append(res, knot)
			orch.comms = append(orch.comms, knot[0], knot[1])
		} else {
			cnt++
			remaining = append(remaining, seg)
		}
	}
	orch.Segs = remaining[:cnt]
	orch.comms = nil
	return res
}

func (orch Orchestrator) harmoniousKnot(knot primitive.Knot) bool {
	for _, comm := range orch.comms {
		if knot[0].Conflict(comm) || knot[1].Conflict(comm) {
			return false
		}
	}
	return true
}
