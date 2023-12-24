package scheduler

import (
	"github.com/google/syzkaller/pkg/interleaving"
)

// TODO: Assumptions and models that this implementation relies on
// (e.g., timestamps represents PO) are so fragile so it is hard to
// extend, for example, to three threads.

type Knotter struct {
	loopAllowed []int
	commChan    map[uint32]struct{}
	accessMap   map[uint32][]interleaving.Access
	numThr      int

	// map: access IDs --> Lock IDs
	locks map[uint32][]int
	// map: access IDs -> chunk IDs
	storeChunks map[uint32]int
	loadChunks  map[uint32]int

	commHsh    map[uint64]struct{}
	windowSize []int

	// input
	seqCount int
	seqs0    [][]interleaving.SerialAccess // Unmodified input
	seqs     [][]interleaving.SerialAccess // Used internally
	// output
	knots map[uint64][]interleaving.Knot
	comms []interleaving.Communication
}

func (knotter *Knotter) AddSequentialTrace(seq []interleaving.SerialAccess) bool {
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

func (knotter *Knotter) sanitizeSequentialTrace(seq []interleaving.SerialAccess) bool {
	if len(seq) <= 1 {
		// 1) Reject a case that a thread does not touch memory at all. In
		// theory, there can be a case that a thread or all threads do not
		// touch any memory objects. We don't need to consider those cases
		// since they do not race anyway. 2) or a single thread is given
		return false
	}
	chk := make([]bool, len(seq))
	for _, serial := range seq {
		if len(serial) == 0 {
			return false
		}
		if !serial.SingleThread() {
			// All serial execution should be a single thread
			return false
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
	knotter.buildAccessMap()
	knotter.annotateLocks()
	knotter.chunknizeSerials()
	knotter.formCommunications()
	knotter.formKnots()
}

func (knotter *Knotter) collectCommChans() {
	knotter.seqs = make([][]interleaving.SerialAccess, len(knotter.seqs0))
	for i, seq := range knotter.seqs0 {
		knotter.seqs[i] = make([]interleaving.SerialAccess, len(seq))
	}

	// Only memory objects on which store operations take place can be
	// a communication channel
	knotter.commChan = make(map[uint32]struct{})
	doSerial := func(f func(*interleaving.SerialAccess, *interleaving.SerialAccess)) {
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

func (knotter *Knotter) collectCommChansSerial(serial, unused *interleaving.SerialAccess) {
	for _, acc := range *serial {
		if acc.Typ == interleaving.TypeStore {
			addr := wordify(acc.Addr)
			knotter.commChan[addr] = struct{}{}
		}
	}
}

func (knotter *Knotter) distillSerial(serial *interleaving.SerialAccess, distiled *interleaving.SerialAccess) {
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

func (knotter *Knotter) buildAccessMap() {
	knotter.accessMap = make(map[uint32][]interleaving.Access)
	for _, seq := range knotter.seqs {
		for _, serial := range seq {
			knotter.buildAccessMapSerial(serial)
		}
	}
}

func (knotter *Knotter) buildAccessMapSerial(serial interleaving.SerialAccess) {
	for _, acc := range serial {
		addr := wordify(acc.Addr)
		knotter.accessMap[addr] = append(knotter.accessMap[addr], acc)
	}
}

func (knotter *Knotter) annotateLocks() {
	// TODO
}

func (knotter *Knotter) chunknizeSerials() {
	// TOOD
}

func (knotter *Knotter) formCommunications() {
	knotter.comms = []interleaving.Communication{}
	knotter.commHsh = make(map[uint64]struct{})
	for _, accs := range knotter.accessMap {
		knotter.formCommunicationAddr(accs)
	}
}

func (knotter *Knotter) formCommunicationAddr(accesses []interleaving.Access) {
	for i := 0; i < len(accesses); i++ {
		for j := i + 0; j < len(accesses); j++ {
			acc0, acc1 := accesses[i], accesses[j]
			if acc0.Thread == acc1.Thread {
				continue
			}

			if acc0.Timestamp > acc1.Timestamp {
				acc0, acc1 = acc1, acc0
			}

			// NOTE: We want to form a communication when one stores a
			// value and the other loads the value. However, all
			// RMW-atomics such that atomic_inc and atomic_dec have
			// the store type, so there is no load even if one atomic
			// in fact reads a value from another atomic. To handle
			// the cases, we discasd cases only when both accesses
			// have the load type.
			if (acc0.Typ == interleaving.TypeLoad) && (acc1.Typ == interleaving.TypeLoad) {
				continue
			}

			if !acc0.Overlapped(acc1) {
				continue
			}

			if knotter.lockContending(acc0, acc1) {
				continue
			}

			knotter.formCommunicationSingle(acc0, acc1)
		}
	}
}

func (knotter *Knotter) lockContending(acc0, acc1 interleaving.Access) bool {
	l0 := knotter.locks[acc0.Inst]
	l1 := knotter.locks[acc1.Inst]
	// TODO: Possibly making a map is too heavy for this.
	ht := make(map[int]struct{})
	for _, l := range l0 {
		ht[l] = struct{}{}
	}
	for _, l := range l1 {
		if _, ok := ht[l]; ok {
			return true
		}
	}
	return false
}

func (knotter *Knotter) formCommunicationSingle(acc0, acc1 interleaving.Access) {
	comm := interleaving.Communication{acc0, acc1}
	if knotter.duppedComm(comm) {
		return
	}
	knotter.comms = append(knotter.comms, comm)
}

func (knotter *Knotter) duppedComm(comm interleaving.Communication) bool {
	// A communication is redundant if there is another that accesses
	// have the same timestamp with corresponding accesses.
	hsh := comm.Hash()
	_, ok := knotter.commHsh[hsh]
	knotter.commHsh[hsh] = struct{}{}
	return ok
}

func (knotter *Knotter) formKnots() {
	knotter.knots = make(map[uint64][]interleaving.Knot)
	knotter.doFormKnotsParallel()
}

func (knotter *Knotter) doFormKnotsParallel() {
	comms := knotter.comms
	for i := 0; i < len(comms); i++ {
		for j := i + 1; j < len(comms); j++ {
			comm0, comm1 := comms[i], comms[j]
			if knotter.canTestMissingStoreBarrier(comm0, comm1) {
				knotter.formKnotForStoreBarrier(comm0, comm1)
			}
			if knotter.canTestMissingLoadBarrier(comm0, comm1) {
				knotter.formKnotForLoadBarrier(comm0, comm1)
			}
		}
	}
}

func (knotter *Knotter) canTestMissingStoreBarrier(comm0, comm1 interleaving.Communication) bool {
	return knotter.inSameChunk(comm0.Former(), comm1.Former(), true)
}

func (knotter *Knotter) canTestMissingLoadBarrier(comm0, comm1 interleaving.Communication) bool {
	return knotter.inSameChunk(comm0.Latter(), comm1.Latter(), false)
}

func (knotter *Knotter) inSameChunk(acc0, acc1 interleaving.Access, storeChunk bool) bool {
	var c0, c1 int
	var ok0, ok1 bool
	if storeChunk {
		c0, ok0 = knotter.storeChunks[acc0.Inst]
		c1, ok1 = knotter.storeChunks[acc1.Inst]
	} else {
		c0, ok0 = knotter.loadChunks[acc0.Inst]
		c1, ok1 = knotter.loadChunks[acc1.Inst]
	}
	if !ok0 || !ok1 {
		return false
	}
	return c0 == c1
}

func (knotter *Knotter) formKnotForStoreBarrier(comm0, comm1 interleaving.Communication) {
	knotter.formKnotSingle(comm0, comm1, true)
}

func (knotter *Knotter) formKnotForLoadBarrier(comm0, comm1 interleaving.Communication) {
	knotter.formKnotSingle(comm0, comm1, false)
}

func (knotter *Knotter) formKnotSingle(comm0, comm1 interleaving.Communication, testingStoreBarrier bool) {
	if !comm0.Parallel(comm1) {
		panic("want parallel but comms are not parallel")
	}
	if comm0.Former().Timestamp > comm1.Former().Timestamp {
		comm0, comm1 = comm1, comm0
	}
	knotter.formKnotSingleSorted(comm0, comm1)
}

func (knotter *Knotter) formKnotSingleSorted(comm0, comm1 interleaving.Communication) {
	knot := interleaving.Knot{comm0, comm1}
	hsh := comm1.Hash()
	knotter.knots[hsh] = append(knotter.knots[hsh], knot)
}

func wordify(addr uint32) uint32 {
	return addr & ^uint32(7)
}

// TODO: Currently QEMU cannot handle multiple dynamic instances, so
// we do not handle them.
// var loopAllowed = []int{1, 2, 4, 8, 16, 32}
var loopAllowed = []int{1}
