package scheduler

import (
	"sort"

	"github.com/google/syzkaller/pkg/primitive"
)

// XXX: using mutiple maps incurs lots of memory allocations which
// slows down ExcavateKnots().
type Knotter struct {
	loopAllowed []int
	commChan    map[uint32]struct{}
	accessMap   map[uint32][]primitive.Access
	numThr      int

	// Used only for thread works. Our implmenetation requires to
	// distinguish two accesses will be executed in different threads,
	// while all Access have same Thread when sequentially executing
	// all calls. When reassignThreadID is true, Knotter will reassign
	// Thread to each Access when fastening Knots
	reassignThreadID bool

	// input
	seqCount int
	seqs0    [][]primitive.SerialAccess // Unmodified input
	seqs     [][]primitive.SerialAccess // Used internally
	// output
	knots []primitive.Segment
	comms []primitive.Segment
}

func (knotter *Knotter) ReassignThreadID() {
	knotter.reassignThreadID = true
}

func (knotter *Knotter) AddSequentialTrace(seq []primitive.SerialAccess) bool {
	if knotter.seqCount == 2 {
		// NOTE: In this project, we build knots using at most two
		// sequential executions. Adding more sequential execution is
		// not allowed.
		return false
	}
	if !knotter.sanitizeSequentialTrace(seq) {
		return false
	}
	knotter.seqs0 = append(knotter.seqs0, seq)
	knotter.seqCount++
	return true
}

func (knotter *Knotter) sanitizeSequentialTrace(seq []primitive.SerialAccess) bool {
	if len(seq) <= 1 {
		// 1) Reject a case that a thread does not touch memory at all. In
		// theory, there can be a case that a thread or all threads do not
		// touch any memory objects. We don't need to consider those cases
		// since they do not race anyway. 2) or a single thread is given
		return false
	}
	var chk []bool
	if !knotter.reassignThreadID {
		chk = make([]bool, len(seq))
	}
	for _, serial := range seq {
		if len(serial) == 0 {
			return false
		}
		if !serial.SingleThread() {
			// All serial execution should be a single thread
			return false
		}
		if knotter.reassignThreadID {
			continue
		}
		thr := int(serial[0].Thread)
		if thr >= len(chk) {
			// thread id should be consecutive starting from 0
		}
		if chk[thr] {
			// All serial should have a different thread id
			return false
		}
		chk[thr] = true
	}
	// NOTE: At this point we take consider cases that all sequential
	// executions have the same nubmer of threads
	if knotter.numThr == 0 {
		knotter.numThr = len(seq)
	} else if knotter.numThr != len(seq) {
		return false
	}
	return true
}

func (knotter *Knotter) ExcavateKnots() {
	if knotter.seqCount < 1 {
		return
	}
	knotter.loopAllowed = loopAllowed
	knotter.fastenKnots()
}

func (knotter *Knotter) fastenKnots() {
	knotter.collectCommChans()
	knotter.inferProgramOrder()
	knotter.buildAccessMap()
	knotter.formCommunications()
	knotter.formKnots()
}

func (knotter *Knotter) collectCommChans() {
	knotter.seqs = make([][]primitive.SerialAccess, len(knotter.seqs0))
	for i, seq := range knotter.seqs0 {
		knotter.seqs[i] = make([]primitive.SerialAccess, len(seq))
	}

	// Only memory objects on which store operations take place can be
	// a communication channel
	knotter.commChan = make(map[uint32]struct{})
	doSerial := func(f func(*primitive.SerialAccess, *primitive.SerialAccess)) {
		for i := 0; i < len(knotter.seqs0); i++ {
			for j := 0; j < len(knotter.seqs0[i]); j++ {
				src := &knotter.seqs0[i][j]
				dst := &knotter.seqs[i][j]
				f(src, dst)
			}
		}
	}
	// Firstly, collect all possible communicatino channels
	doSerial(knotter.collectCommChansSerial)
	// Then, distill all serial accesses
	doSerial(knotter.distillSerial)
}

func (knotter *Knotter) collectCommChansSerial(serial, unused *primitive.SerialAccess) {
	for _, acc := range *serial {
		if acc.Typ == primitive.TypeStore {
			addr := wordify(acc.Addr)
			knotter.commChan[addr] = struct{}{}
		}
	}
}

func (knotter *Knotter) distillSerial(serial *primitive.SerialAccess, distiled *primitive.SerialAccess) {
	loopCnt := make(map[uint32]int)
	for _, acc := range *serial {
		addr := wordify(acc.Addr)
		if _, ok := knotter.commChan[addr]; !ok {
			continue
		}
		loopCnt[acc.Inst]++
		// TODO: this loop can be optimized
		for _, allowed := range knotter.loopAllowed {
			if allowed == loopCnt[acc.Inst] {
				(*distiled) = append((*distiled), acc)
				break
			}
		}
	}
}

func (knotter *Knotter) inferProgramOrder() {
	if knotter.seqCount == 1 {
		// If we have only one sequential execution, ts timestamps
		// represent the program order as is.
		return
	}

	if knotter.reassignThreadID {
		panic("not yet handled") // And probably will not be handled
	}

	for i := 0; i < knotter.numThr; i++ {
		serials := knotter.pickThread(uint64(i))
		knotter.alignThread(serials)
	}
}

func (knotter *Knotter) pickThread(id uint64) []primitive.SerialAccess {
	thr := []primitive.SerialAccess{}
	for i := range knotter.seqs {
		for j := range knotter.seqs[i] {
			serial := knotter.seqs[i][j]
			if serial[0].Thread == id {
				thr = append(thr, serial)
				break
			}
		}
	}
	return thr
}

func (knotter *Knotter) alignThread(thr []primitive.SerialAccess) {
	if len(thr) < 2 {
		return
	}
	pairwiseSequenceAlign(&thr[0], &thr[1])
}

func (knotter *Knotter) buildAccessMap() {
	// Record specific dynamic instances for the same instruction to
	// handle loops
	knotter.accessMap = make(map[uint32][]primitive.Access)
	for _, seq := range knotter.seqs {
		for _id, serial := range seq {
			if len(serial) == 0 {
				continue
			}
			id := serial[0].Thread
			if knotter.reassignThreadID {
				id = uint64(_id)
			}
			knotter.buildAccessMapSerial(serial, id)
		}
	}
}

func (knotter *Knotter) buildAccessMapSerial(serial primitive.SerialAccess, id uint64) {
	for _, acc := range serial {
		addr := wordify(acc.Addr)
		if _, ok := knotter.commChan[addr]; !ok {
			continue
		}
		acc.Thread = id
		knotter.accessMap[addr] = append(knotter.accessMap[addr], acc)
	}
}

func (knotter *Knotter) formCommunications() {
	knotter.comms = []primitive.Segment{}
	for _, accs := range knotter.accessMap {
		knotter.formCommunicationAddr(accs)
	}
}

func (knotter *Knotter) formCommunicationAddr(accesses []primitive.Access) {
	for i := 0; i < len(accesses); i++ {
		for j := i + 1; j < len(accesses); j++ {
			acc1, acc2 := accesses[i], accesses[j]
			if acc1.Thread == acc2.Thread {
				continue
			}

			// NOTE: We want to form a communication when one stores a
			// value and the other loads the value. However, all
			// RMW-atomics such that atomic_inc and atomic_dec have
			// the store type, so there is no load even if one atomic
			// in fact reads a value from another atomic. To handle
			// the cases, we discasd cases only when both accesses
			// have the load type.
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

func (knotter *Knotter) formKnots() {
	knotter.knots = []primitive.Segment{}
	for i := 0; i < len(knotter.comms); i++ {
		for j := i + 1; j < len(knotter.comms); j++ {
			comm1, comm2 := knotter.comms[i].(primitive.Communication), knotter.comms[j].(primitive.Communication)
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

func (knotter *Knotter) GetCommunications() []primitive.Segment {
	return knotter.comms
}

func (knotter *Knotter) GetKnots() []primitive.Segment {
	return knotter.knots
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

func wordify(addr uint32) uint32 {
	return addr & ^uint32(7)
}

// TODO: Currently QEMU cannot handle multiple dynamic instances, so
// we do not handle them.
// var loopAllowed = []int{1, 2, 4, 8, 16, 32}
var loopAllowed = []int{1}
