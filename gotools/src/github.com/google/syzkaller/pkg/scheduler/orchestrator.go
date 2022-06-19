package scheduler

import "github.com/google/syzkaller/pkg/interleaving"

type Orchestrator struct {
	// Communications that are already selected
	comms []interleaving.Communication
	// Input knots
	Segs []interleaving.Segment
	Used []interleaving.Segment
}

// TODO: The time complexity of orchestrator.SelectHarmoniousKnots()
// is O(n*n). Reduce it to O(n).

func (orch *Orchestrator) SelectHarmoniousKnots() []interleaving.Knot {
	res := []interleaving.Knot{}
	remaining := make([]interleaving.Segment, 0, len(orch.Segs))
	cnt := 0
	for _, seg := range orch.Segs {
		if knot, ok := seg.(interleaving.Knot); ok && orch.harmoniousKnot(knot) {
			res = append(res, knot)
			orch.comms = append(orch.comms, knot[0], knot[1])
			orch.Used = append(orch.Used, knot)
		} else {
			cnt++
			remaining = append(remaining, seg)
		}
	}
	orch.Segs = remaining[:cnt]
	orch.comms = nil
	return res
}

func (orch Orchestrator) harmoniousKnot(knot interleaving.Knot) bool {
	for _, comm := range orch.comms {
		if knot[0].Conflict(comm) || knot[1].Conflict(comm) {
			return false
		}
	}
	return true
}
