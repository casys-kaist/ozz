package scheduler

import (
	"sort"

	"github.com/google/syzkaller/pkg/primitive"
)

// TODO: Assumptions and models that this implementation relies on
// (e.g., timestamps represents PO) are so fragile so it is hard to
// extend, for example, to three threads.

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
	ReassignThreadID bool

	commHsh map[uint64]struct{}
	knotHsh map[uint64]struct{}

	// input
	seqCount int
	seqs0    [][]primitive.SerialAccess // Unmodified input
	seqs     [][]primitive.SerialAccess // Used internally
	// output
	knots []primitive.Segment
	comms []primitive.Segment
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
	if !knotter.ReassignThreadID {
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
		if knotter.ReassignThreadID {
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
	// At this point, two accesses conducted by a single thread are
	// same if they have the same timestamp
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
		// Deal with specific dynamic instances for the same instruction
		// to handle loops
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

	if knotter.ReassignThreadID {
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
			if len(serial) == 0 {
				continue
			}
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
	knotter.accessMap = make(map[uint32][]primitive.Access)

	for _, seq := range knotter.seqs {
		for _id, serial := range seq {
			if len(serial) == 0 {
				continue
			}
			id := serial[0].Thread
			if knotter.ReassignThreadID {
				id = uint64(_id)
			}
			knotter.buildAccessMapSerial(serial, id)
		}
	}
	// Communication channels will not be used after this point. Let's
	// nil it for the garbage collector to know it.
	knotter.commChan = nil
}

func (knotter *Knotter) buildAccessMapSerial(serial primitive.SerialAccess, id uint64) {
	for _, acc := range serial {
		addr := wordify(acc.Addr)
		acc.Thread = id
		knotter.accessMap[addr] = append(knotter.accessMap[addr], acc)
	}
}

func (knotter *Knotter) formCommunications() {
	knotter.comms = []primitive.Segment{}
	knotter.commHsh = make(map[uint64]struct{})
	for _, accs := range knotter.accessMap {
		knotter.formCommunicationAddr(accs)
	}
	// As same to knotter.commChan, let's nil it for GC.
	knotter.accessMap = nil
	knotter.commHsh = nil
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
			candidates := []primitive.Communication{{acc1, acc2}, {acc2, acc1}}
			for _, cand := range candidates {
				if knotter.duppedComm(cand) {
					continue
				}
				knotter.comms = append(knotter.comms, cand)
			}
		}
	}
}

func (knotter *Knotter) duppedComm(comm primitive.Communication) bool {
	// A communication is redundant if there is another that accesses
	// have the same timestamp with corresponding accesses.
	hsh := comm.Hash()
	_, ok := knotter.commHsh[hsh]
	knotter.commHsh[hsh] = struct{}{}
	return ok
}

func (knotter *Knotter) formKnots() {
	knotter.knots = []primitive.Segment{}
	knotter.knotHsh = make(map[uint64]struct{})
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
			if knotter.duppedKnot(knot) {
				continue
			}
			knotter.knots = append(knotter.knots, knot)
		}
	}
}

func (knotter *Knotter) duppedKnot(knot primitive.Knot) bool {
	hsh := knot.Hash()
	_, ok := knotter.knotHsh[hsh]
	knotter.knotHsh[hsh] = struct{}{}
	return ok
}

func (knotter *Knotter) GetCommunications() []primitive.Segment {
	return knotter.comms
}

func (knotter *Knotter) GetKnots() []primitive.Segment {
	return knotter.knots
}

type Scheduler struct {
	// input
	Knots []primitive.Knot
	// output
	schedPoints []primitive.Access
}

func (sched *Scheduler) GenerateSchedPoints() ([]primitive.Access, bool) {
	dag := sched.buildDAG()
	nodes, ok := dag.topologicalSort()
	if !ok {
		return nil, false
	}
	for _, node := range nodes {
		acc := node.(primitive.Access)
		sched.schedPoints = append(sched.schedPoints, primitive.Access(acc))
	}
	return sched.schedPoints, true
}

func (sched *Scheduler) SqueezeSchedPoints() []primitive.Access {
	new := []primitive.Access{}
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
	for i /*, knot */ := range sched.Knots {
		for j /*, comm */ := range sched.Knots[i] {
			former := sched.Knots[i][j].Former()
			latter := sched.Knots[i][j].Latter()
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
