package scheduler

import (
	"sort"

	"github.com/google/syzkaller/pkg/primitive"
)

// TODO: move StaticAccess to the primitive package
type StaticAccess struct {
	Inst   uint32
	Thread uint64
}

type knotter struct {
	loopAllowed []int
	loopCnt     map[StaticAccess]int
	accessMap   map[uint32][]primitive.Access
	comms       []primitive.Communication
	// input
	accesses []primitive.SerialAccess
	// output
	knots []primitive.Knot
}

// TODO: Currently QEMU cannot handle multiple dynamic instances, so
// we do not handle them.
// var loopAllowed = []int{1, 2, 4, 8, 16, 32}
var loopAllowed = []int{1}

func ExcavateKnots(accesses []primitive.SerialAccess) []primitive.Knot {
	knotter := knotter{
		accesses:    accesses,
		loopAllowed: loopAllowed,
	}
	knotter.fastenKnots()
	return knotter.knots
}

func (knotter *knotter) fastenKnots() {
	knotter.buildAccessMap()
	knotter.formCommunications()
	knotter.formKnots()
}

func (knotter *knotter) buildAccessMap() {
	// XXX: using maps incurs lots of memory allocations which slows
	// down ExcavatgeKnots().

	// 1) accessMap do not need to contain accesses for addresses on
	// which only loads are taken. 2) record specific dynamic
	// instances for the same instruction to handle loops
	knotter.accessMap = make(map[uint32][]primitive.Access)
	knotter.loopCnt = make(map[StaticAccess]int)

	// step1: record all writes
	knotter.pickAccessesCond(func(acc primitive.Access) bool {
		return acc.Typ == primitive.TypeStore
	})

	// step 2: record loads that have corresponding writes
	knotter.pickAccessesCond(func(acc primitive.Access) bool {
		addr := acc.Addr & ^uint32(7)
		_, ok := knotter.accessMap[addr]
		return acc.Typ == primitive.TypeLoad && ok
	})
}

func (knotter *knotter) pickAccessesCond(cond func(acc primitive.Access) bool) {
	for _, accs := range knotter.accesses {
		for _, acc := range accs {
			if !cond(acc) {
				continue
			}
			sa := StaticAccess{Inst: acc.Inst, Thread: acc.Thread}
			knotter.loopCnt[sa]++
			// TODO: this loop can be optimized
			for _, allowed := range knotter.loopAllowed {
				if allowed == knotter.loopCnt[sa] {
					addr := acc.Addr & ^uint32(7)
					knotter.accessMap[addr] = append(knotter.accessMap[addr], acc)
					break
				}
			}
		}
	}
}

func (knotter *knotter) formCommunications() {
	knotter.comms = []primitive.Communication{}
	for _, accs := range knotter.accessMap {
		knotter.formCommunicationAddr(accs)
	}
}

func (knotter *knotter) formCommunicationAddr(accesses []primitive.Access) {
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
			knotter.comms = append(knotter.comms, primitive.Communication{acc1, acc2}, primitive.Communication{acc2, acc1})
		}
	}
}

func (knotter *knotter) formKnots() {
	knotter.knots = []primitive.Knot{}
	for i := 0; i < len(knotter.comms); i++ {
		for j := i + 1; j < len(knotter.comms); j++ {
			comm1, comm2 := knotter.comms[i], knotter.comms[j]
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

func (sched *Scheduler) GenerateSchedPoints() ([]SchedPoint, bool) {
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

func (sched *Scheduler) SqueezeSchedPoints() []SchedPoint {
	new := []SchedPoint{}
	preempted := make(map[uint64]bool)
	for i := range sched.schedPoints {
		if preempted[sched.schedPoints[i].Thread] {
			// This is the first instruction after the thread is
			// preempted. This should be a sched point
			preempted[sched.schedPoints[i].Thread] = false
			new = append(new, sched.schedPoints[i])
		}
		if i == len(sched.schedPoints)-1 || sched.schedPoints[i].Thread != sched.schedPoints[i+1].Thread {
			preempted[sched.schedPoints[i].Thread] = true
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
