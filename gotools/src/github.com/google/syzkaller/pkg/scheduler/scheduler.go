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
	commHsh     map[uint64]struct{}

	// Per thread map (access IDs --> Lock IDs)
	locks []map[uint32][]int
	// Per thread map (access IDs -> chunk IDs)
	storeChunks []map[uint32]int
	loadChunks  []map[uint32]int

	// input
	seq0 []interleaving.SerialAccess // Unmodified input
	seq  []interleaving.SerialAccess // Used internally
	// output
	knots map[uint64][]interleaving.Knot
	comms []interleaving.Communication
	// Sets of knot hashes.
	delayingStores   map[uint64]struct{}
	prefetchingLoads map[uint64]struct{}
}

func (knotter *Knotter) AddSequentialTrace(seq []interleaving.SerialAccess) bool {
	if len(seq) != 2 {
		return false
	}
	knotter.seq0 = seq
	return true
}

func (knotter *Knotter) ExcavateKnots() {
	if knotter.seq == nil {
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
	knotter.postProcessing()
}

func (knotter *Knotter) collectCommChans() {
	// Only memory objects on which store operations take place can be
	// a communication channel
	knotter.seq = make([]interleaving.SerialAccess, len(knotter.seq0))
	knotter.commChan = make(map[uint32]struct{})
	doSerial := func(f func(*interleaving.SerialAccess, *interleaving.SerialAccess)) {
		for i := 0; i < len(knotter.seq0); i++ {
			src := &knotter.seq0[i]
			dst := &knotter.seq[i]
			f(src, dst)
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
		if typ := acc.Typ; typ == interleaving.TypeLoad || typ == interleaving.TypeStore {
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
		} else {
			(*distiled) = append((*distiled), acc)
		}
	}
}

func (knotter *Knotter) buildAccessMap() {
	knotter.accessMap = make(map[uint32][]interleaving.Access)
	for _, serial := range knotter.seq {
		knotter.buildAccessMapSerial(serial)
	}
}

func (knotter *Knotter) buildAccessMapSerial(serial interleaving.SerialAccess) {
	for _, acc := range serial {
		addr := wordify(acc.Addr)
		knotter.accessMap[addr] = append(knotter.accessMap[addr], acc)
	}
}

func (knotter *Knotter) annotateLocks() {
	knotter.locks = make([]map[uint32][]int, 2)
	for _, serial := range knotter.seq {
		if len(serial) == 0 {
			continue
		}
		tid := serial[0].Thread
		knotter.annotateLocksInSerial(uint32(tid), serial)
	}
}

func (knotter *Knotter) annotateLocksInSerial(tid uint32, serial interleaving.SerialAccess) {
	knotter.locks[tid] = make(map[uint32][]int)
	// NOTE: Unless there is a deadlock, traces of lock operations are
	// always a form of a stack.
	locks, head := make([]int, 16), 0
loop:
	for _, acc := range serial {
		switch typ := acc.Typ; typ {
		case interleaving.TypeLoad, interleaving.TypeStore:
			memID := getMemID(acc)
			knotter.locks[tid][memID] = append([]int{}, locks[:head]...)
		case interleaving.TypeLockAcquire:
			lockID := getLockID(acc)
			locks[head] = lockID
			head++
		case interleaving.TypeLockRelease:
			lockID := getLockID(acc)
			// NOTE: KMemcov does not record if a lock acquire is
			// try-lock. This causes a trace to miss lock acquires
			// even if they are successful, leading an incomplete
			// trace.
			for i := head - 1; i >= 0; i-- {
				if lockID == locks[i] {
					head = i
					continue loop
				}
			}
			head = 0
		}
	}
}

func (knotter *Knotter) chunknizeSerials() {
	knotter.storeChunks = make([]map[uint32]int, 2)
	knotter.loadChunks = make([]map[uint32]int, 2)
	for _, serial := range knotter.seq {
		if len(serial) == 0 {
			continue
		}
		tid := uint32(serial[0].Thread)
		knotter.chunknizeSerial(tid, serial)
	}
}

func (knotter *Knotter) chunknizeSerial(tid uint32, serial interleaving.SerialAccess) {
	knotter.storeChunks[tid] = make(map[uint32]int)
	knotter.loadChunks[tid] = make(map[uint32]int)
	storeChunkID, loadChunkID := 0, 0
	for _, acc := range serial {
		switch typ := acc.Typ; typ {
		case interleaving.TypeStore, interleaving.TypeLoad:
			memId := getMemID(acc)
			knotter.storeChunks[tid][memId] = storeChunkID
			knotter.loadChunks[tid][memId] = loadChunkID
		case interleaving.TypeFlush:
			storeChunkID++
		case interleaving.TypeLFence:
			loadChunkID++
		}
	}
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
	l0 := knotter.locks[uint32(acc0.Thread)][acc0.Inst]
	l1 := knotter.locks[uint32(acc1.Thread)][acc1.Inst]
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
	comms := knotter.comms
	for i := 0; i < len(comms); i++ {
		for j := i + 1; j < len(comms); j++ {
			comm0, comm1, ok := canonicalize(comms, i, j)
			if !ok {
				continue
			}
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
	if acc0.Thread != acc1.Thread {
		panic("wrong")
	}
	var c0, c1 int
	var ok0, ok1 bool
	tid := uint32(acc0.Thread)
	if storeChunk {
		c0, ok0 = knotter.storeChunks[tid][acc0.Inst]
		c1, ok1 = knotter.storeChunks[tid][acc1.Inst]
	} else {
		c0, ok0 = knotter.loadChunks[tid][acc0.Inst]
		c1, ok1 = knotter.loadChunks[tid][acc1.Inst]
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
	knot := interleaving.Knot{comm0, comm1}
	critHsh := comm1.Hash()
	knotHsh := knot.Hash()
	knotter.knots[critHsh] = append(knotter.knots[critHsh], knot)
	if testingStoreBarrier {
		knotter.delayingStores[knotHsh] = struct{}{}
	} else {
		knotter.prefetchingLoads[knotHsh] = struct{}{}
	}
}

func (knotter *Knotter) postProcessing() {
	// XXX: I'm not sure this is helpful. Intuitively, if we can test
	// a knot for both cases, it is possibly enough to test it for one
	// among them.
	for hsh := range knotter.testingStoreBarrier {
		delete(knotter.testingLoadBarrier, hsh)
	}
}

func canonicalize(comms []interleaving.Communication, i, j int) (comm0, comm1 interleaving.Communication, ok bool) {
	if comm0.Former().Timestamp > comm1.Former().Timestamp {
		comm0, comm1 = comm1, comm0
	}
	// We don't need to test !ok case as it can be tested as a normal
	// race condition
	ok = !(comm0.Latter().Timestamp < comm1.Latter().Timestamp)
	return
}

func wordify(addr uint32) uint32 {
	return addr & ^uint32(7)
}

func getMemID(acc interleaving.Access) uint32 {
	return acc.Inst
}

func getLockID(acc interleaving.Access) int {
	return int(acc.Addr)
}

// TODO: Currently QEMU cannot handle multiple dynamic instances, so
// we do not handle them.
// var loopAllowed = []int{1, 2, 4, 8, 16, 32}
var loopAllowed = []int{1}
