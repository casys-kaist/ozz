package scheduler

import (
	"sort"

	"github.com/google/syzkaller/pkg/primitive"
)

// NOTE: Communicadtion[0] must/will happen before Communication[1]
// NOTE: Assumption: Accesses's timestamps in SerialAccess have
// the same order as the program order
type Communication [2]primitive.Access

func (comm *Communication) Former() primitive.Access {
	return comm[0]
}

func (comm *Communication) Latter() primitive.Access {
	return comm[1]
}

func (comm Communication) Same(comm0 Communication) bool {
	return comm[0].Inst == comm0[0].Inst && comm[1].Inst == comm0[1].Inst
}

func (comm0 Communication) Conflict(comm1 Communication) bool {
	if comm0.Parallel(comm1) {
		// Parallell comms cannot be conflict
		return false
	}
	return comm0[1].Timestamp <= comm1[0].Timestamp && comm1[1].Timestamp <= comm0[0].Timestamp ||
		comm1[1].Timestamp <= comm0[0].Timestamp && comm0[1].Timestamp <= comm1[0].Timestamp
}

func (comm0 Communication) Parallel(comm1 Communication) bool {
	for i := 0; i < 2; i++ {
		if comm0[i].Thread != comm1[i].Thread {
			return false
		}
	}
	return true
}

func (comm0 Communication) HappenBefore(comm1 Communication) bool {
	return comm0[1].Timestamp < comm1[0].Timestamp &&
		comm0[0].Timestamp < comm1[1].Timestamp
}

type Knot [2]Communication

func (knot Knot) Same(knot0 Knot) bool {
	return knot[0].Same(knot0[0]) && knot[1].Same(knot0[1])
}

type KnotType int

const (
	KnotInvalid KnotType = iota
	KnotParallel
	KnotOverlapped
	KnotSeparated
)

type knotter struct {
	accesses []primitive.SerialAccess
	knots    []Knot
}

func ExcavateKnots(accesses []primitive.SerialAccess) []Knot {
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

	comms := []Communication{}
	for _, accs := range mp {
		comms = append(comms, formCommunication(accs)...)
	}
	knotter.formKnots(comms)
}

func formCommunication(accesses []primitive.Access) []Communication {
	comms := []Communication{}
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
			comms = append(comms, Communication{acc1, acc2}, Communication{acc2, acc1})
		}
	}
	return comms
}

func (knotter *knotter) formKnots(comms []Communication) {
	for i := 0; i < len(comms); i++ {
		for j := i + 1; j < len(comms); j++ {
			comm1, comm2 := comms[i], comms[j]
			if comm1[0].Timestamp > comm2[0].Timestamp {
				comm1, comm2 = comm2, comm1
			}
			knot := Knot{comm1, comm2}
			if typ := knot.Type(); typ == KnotParallel || typ == KnotInvalid {
				continue
			}
			knotter.knots = append(knotter.knots, knot)
		}
	}
}

func (knot Knot) Type() KnotType {
	comm0, comm1 := knot[0], knot[1]
	if comm0.Parallel(comm1) {
		return KnotParallel
	} else if comm0.Conflict(comm1) || comm1.Conflict(comm0) {
		// Invalid program orders
		return KnotInvalid
	} else if comm0.HappenBefore(comm1) || comm1.HappenBefore(comm0) {
		return KnotSeparated
	} else {
		return KnotOverlapped
	}
}

type orchestrator struct {
	// Communications that are already selected
	comms []Communication
	// Input knots
	knots []Knot
}

func (orch *orchestrator) selectHarmoniousKnots() []Knot {
	res := []Knot{}
	remaining := make([]Knot, 0, len(orch.knots))
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

func (orch orchestrator) harmoniousKnot(knot Knot) bool {
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
	knots []Knot
	// output
	schedPoints []SchedPoint
}

func (sched Scheduler) GenerateSchedPoints() []SchedPoint {
	dag := sched.buildDAG()
	for _, node := range dag.topologicalSort() {
		sched.schedPoints = append(sched.schedPoints, SchedPoint(node))
	}
	return sched.schedPoints
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
	d := dag{
		nodes: make(map[node]struct{}),
		edges: make(map[node]map[node]struct{}),
	}
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

type dag struct {
	nodes map[node]struct{}
	edges edge
}

func (d *dag) addEdge(src0, dst0 primitive.Access) {
	src, dst := node(src0), node(dst0)
	d.nodes[src] = struct{}{}
	if _, ok := d.edges[src]; !ok {
		d.edges[src] = make(map[node]struct{})
	}
	d.edges[src][dst] = struct{}{}
}

func (d dag) topologicalSort() []node {
	res := make([]node, 0, len(d.nodes))
	q, head := make([]node, 0, len(d.nodes)), 0
	inbounds := make(map[node]int)
	// Preprocessing: calculating in-bounds
	for v := range d.nodes {
		inbounds[v] = 0
	}

	for _, dsts := range d.edges {
		for dst := range dsts {
			inbounds[dst]++
		}
	}

	// step 1: queue all nodes with 0 inbound
	for n, inbound := range inbounds {
		if inbound == 0 {
			q = append(q, n)
		}
	}

	// step 2: iteratively infd a vertex with 0 inbound
	for head < len(q) {
		v := q[head]
		head++
		res = append(res, v)
		for dst := range d.edges[v] {
			inbounds[dst]--
			if inbounds[dst] == 0 {
				q = append(q, dst)
			}
		}
	}

	return res
}

// TODO: primitive.Access does not contain many fields at this time,
// so comparing two does not incur a high overhead. So we decide to
// use primitive.Access as is as a node. But obviously this introduces
// the unnecessary overhead, so if required, use a pointer of it as a
// node.
type node primitive.Access

type edge map[node]map[node]struct{}
