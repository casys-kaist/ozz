package scheduler

import (
	"sort"

	"github.com/google/syzkaller/pkg/primitive"
)

type knotter struct {
	accesses []primitive.SerialAccess
	knots    []primitive.Knot
}

func ExcavateKnots(accesses []primitive.SerialAccess) []primitive.Knot {
	knotter := knotter{
		accesses: accesses,
	}
	knotter.fastenKnots()
	return knotter.knots
}

func (knotter *knotter) fastenKnots() {
	mp := make(map[uint32][]primitive.Access)
	for _, accs := range knotter.accesses {
		for _, acc := range accs {
			addr := acc.Addr & ^uint32(7)
			mp[addr] = append(mp[addr], acc)
		}
	}

	comms := []primitive.Communication{}
	for _, accs := range mp {
		comms = append(comms, formCommunication(accs)...)
	}
	knotter.formKnots(comms)
}

func formCommunication(accesses []primitive.Access) []primitive.Communication {
	comms := []primitive.Communication{}
	for i := 0; i < len(accesses); i++ {
		for j := i + 1; j < len(accesses); j++ {
			acc1, acc2 := accesses[i], accesses[j]
			if acc1.Thread == acc2.Thread {
				continue
			}

			if acc1.Typ == primitive.TypeLoad && acc2.Typ == primitive.TypeLoad {
				continue
			}

			if !acc1.Overlapped(acc2) {
				continue
			}

			// We are generating all possible knots so append both
			// Communications
			comms = append(comms, primitive.Communication{acc1, acc2}, primitive.Communication{acc2, acc1})
		}
	}
	return comms
}

func (knotter *knotter) formKnots(comms []primitive.Communication) {
	for i := 0; i < len(comms); i++ {
		for j := i + 1; j < len(comms); j++ {
			comm1, comm2 := comms[i], comms[j]
			if comm1[0].Timestamp > comm2[0].Timestamp {
				comm1, comm2 = comm2, comm1
			}
			knot := primitive.Knot{comm1, comm2}
			if typ := knot.Type(); typ == primitive.KnotParallel || typ == primitive.KnotInvalid {
				continue
			}
			knotter.knots = append(knotter.knots, knot)
		}
	}
}

type orchestrator struct {
	// Communications that are already selected
	comms []primitive.Communication
	// Input knots
	knots []primitive.Knot
}

func (orch *orchestrator) selectHarmoniousKnots() []primitive.Knot {
	res := []primitive.Knot{}
	remaining := make([]primitive.Knot, 0, len(orch.knots))
	for _, knot := range orch.knots {
		if orch.harmoniousKnot(knot) {
			res = append(res, knot)
			orch.comms = append(orch.comms, knot[0], knot[1])
		} else {
			remaining = append(remaining, knot)
		}
	}
	orch.knots = remaining
	orch.comms = nil
	return res
}

func (orch orchestrator) harmoniousKnot(knot primitive.Knot) bool {
	for _, comm := range orch.comms {
		if knot[0].Conflict(comm) || knot[1].Conflict(comm) {
			return false
		}
	}
	return true
}

type SchedPoint primitive.Access

type Scheduler struct {
	// input
	knots []primitive.Knot
	// output
	schedPoints []SchedPoint
}

func (sched Scheduler) GenerateSchedPoints() ([]SchedPoint, bool) {
	dag := sched.buildDAG()
	nodes, ok := dag.topologicalSort()
	if !ok {
		return nil, false
	}
	for _, node := range nodes {
		acc := node.(primitive.Access)
		sched.schedPoints = append(sched.schedPoints, SchedPoint(acc))
	}
	return sched.schedPoints, true
}

func (sched *Scheduler) SquizeSchedPoints() []SchedPoint {
	new := []SchedPoint{}
	for i := range sched.schedPoints {
		if i == len(sched.schedPoints)-1 || sched.schedPoints[i].Thread != sched.schedPoints[i+1].Thread {
			new = append(new, sched.schedPoints[i])
		}
	}

	sched.schedPoints = new
	return sched.schedPoints
}

func (sched *Scheduler) buildDAG() dag {
	d := newDAG()
	threads := make(map[uint64][]primitive.Access)
	for i /*, knot */ := range sched.knots {
		for j /*, comm */ := range sched.knots[i] {
			former := sched.knots[i][j].Former()
			latter := sched.knots[i][j].Latter()
			d.addEdge(former, latter)
			threads[former.Thread] = append(threads[former.Thread], former)
			threads[latter.Thread] = append(threads[latter.Thread], latter)
		}
	}

	for /* threadid*/ _, accs := range threads {
		// NOTE: timestamp represents the program order
		sort.Slice(accs, func(i, j int) bool { return accs[i].Timestamp < accs[j].Timestamp })
		for i /*, acc*/ := range accs {
			if i != len(accs)-1 && accs[i].Timestamp != accs[i+1].Timestamp {
				d.addEdge(accs[i], accs[i+1])
			}
		}
	}
	return d
}
