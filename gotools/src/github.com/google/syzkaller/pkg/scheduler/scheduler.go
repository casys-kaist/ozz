package scheduler

import (
	"sort"
	"time"

	"github.com/google/syzkaller/pkg/interleaving"
)

// TODO: Assumptions and models that this implementation relies on
// (e.g., timestamps represents PO) are so fragile so it is hard to
// extend, for example, to three threads.

type Knotter struct {
	config config
	opts   KnotterOpts

	loopAllowed []int
	commChan    map[uint32]struct{}
	accessMap   map[uint32][]interleaving.Access
	numThr      int

	commHsh        map[uint64]struct{}
	innerCommCount map[interleaving.Communication]int
	comms0         []interleaving.Communication
	comms1         []interleaving.Communication
	windowSize     []int

	// input
	seqCount int
	seqs0    [][]interleaving.SerialAccess // Unmodified input
	seqs     [][]interleaving.SerialAccess // Used internally
	// output
	knots []interleaving.Segment
	comms []interleaving.Segment
}

func GetKnotter(opts KnotterOpts) Knotter {
	if !opts.flagSet(FlagWantParallel) {
		opts.Flags &= ^FlagWantMessagePassing
	}
	return Knotter{
		opts: opts,
	}
}

type config struct {
	factor    float32
	threshold int
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
	var chk []bool
	if !knotter.opts.flagSet(FlagReassignThreadID) {
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
		if knotter.opts.flagSet(FlagReassignThreadID) {
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
	time.Sleep(time.Microsecond * 100)
	knotter.fastenKnots()
}

func (knotter *Knotter) fastenKnots() {
	knotter.collectCommChans()
	knotter.doReassignThreadID()
	knotter.inferProgramOrder()
	knotter.inferWindowSize()
	// At this point, two accesses conducted by a single thread are
	// same if they have the same timestamp
	knotter.buildAccessMap()
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

func (knotter *Knotter) doReassignThreadID() {
	if !knotter.opts.flagSet(FlagReassignThreadID) {
		return
	}
	if len(knotter.seqs) != 1 {
		panic("wrong")
	}
	for id := range knotter.seqs[0] {
		for i := range knotter.seqs[0][id] {
			knotter.seqs[0][id][i].Thread = uint64(id)
		}
	}
}

func (knotter *Knotter) inferProgramOrder() {
	for i := 0; i < knotter.numThr; i++ {
		serials := knotter.pickThread(uint64(i))
		knotter.alignThread(serials)
	}
}

func (knotter *Knotter) inferWindowSize() {
	knotter.windowSize = make([]int, knotter.numThr)
	for tid := 0; tid < knotter.numThr; tid++ {
		serials := knotter.pickThread(uint64(tid))
		size := 0
		for _, serial := range serials {
			if len(serial) == 0 {
				continue
			}
			last := len(serial) - 1
			curSize := int(serial[last].Timestamp - serial[0].Timestamp)
			if size < curSize {
				size = curSize
			}
		}
		knotter.windowSize[tid] = size
	}
}

func (knotter *Knotter) pickThread(id uint64) []interleaving.SerialAccess {
	thr := []interleaving.SerialAccess{}
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

func (knotter *Knotter) alignThread(thr []interleaving.SerialAccess) {
	if len(thr) < 2 {
		return
	}
	pairwiseSequenceAlign(&thr[0], &thr[1])
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

func (knotter *Knotter) formCommunications() {
	knotter.comms = []interleaving.Segment{}
	knotter.commHsh = make(map[uint64]struct{})
	knotter.innerCommCount = make(map[interleaving.Communication]int)
	for _, accs := range knotter.accessMap {
		knotter.formCommunicationAddr(accs)
	}
}

func (knotter *Knotter) formCommunicationAddr(accesses []interleaving.Access) {
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
			if acc1.Typ == interleaving.TypeLoad && acc2.Typ == interleaving.TypeLoad {
				continue
			}

			if !acc1.Overlapped(acc2) {
				continue
			}

			// We are generating all possible knots so append both
			// Communications
			knotter.formCommunicationSingle(acc1, acc2)
			knotter.formCommunicationSingle(acc2, acc1)
		}
	}
}

func (knotter *Knotter) formCommunicationSingle(acc0, acc1 interleaving.Access) {
	if knotter.opts.flagSet(FlagStrictTimestamp) && acc0.Timestamp > acc1.Timestamp {
		return
	}
	comm := interleaving.Communication{acc0, acc1}
	if knotter.duppedComm(comm) {
		return
	}
	knotter.comms = append(knotter.comms, comm)
	if acc0.Thread < acc1.Thread {
		knotter.comms0 = append(knotter.comms0, comm)
		knotter.countInnerCommunication(knotter.comms0, comm)
	} else {
		knotter.comms1 = append(knotter.comms1, comm)
		knotter.countInnerCommunication(knotter.comms1, comm)
	}
}

func (knotter *Knotter) duppedComm(comm interleaving.Communication) bool {
	// A communication is redundant if there is another that accesses
	// have the same timestamp with corresponding accesses.
	hsh := comm.Hash()
	_, ok := knotter.commHsh[hsh]
	knotter.commHsh[hsh] = struct{}{}
	return ok
}

func (knotter *Knotter) countInnerCommunication(comms []interleaving.Communication, comm interleaving.Communication) {
	for _, inner := range comms {
		if inner.Imply(comm) {
			knotter.innerCommCount[comm]++
		}
	}
}

func (knotter *Knotter) formKnots() {
	configs := []config{
		{0.2, 20},
		{0.15, 16},
		{0.1, 13},
		{0.05, 10},
	}
	for step, stop := 0, false; step < 4 && !stop; step++ {
		knotter.config = configs[step]
		stop = knotter.doFormKnots()
	}
}

func (knotter *Knotter) doFormKnots() bool {
	knotter.knots = []interleaving.Segment{}
	if knotter.opts.Mu != nil {
		(*knotter.opts.Mu).RLock()
		defer (*knotter.opts.Mu).RUnlock()
	}

	if knotter.opts.flagSet(FlagWantParallel) {
		// RelRazzer
		knotter.doFormKnotsParallel()
	} else {
		// SegFuzz
		knotter.doFormKnotsNotParallel()
	}
	return len(knotter.knots) < thresholdKnots
}

func (knotter *Knotter) doFormKnotsNotParallel() {
	for _, comm0 := range knotter.comms0 {
		if knotter.tooManyNestedComm(comm0) {
			continue
		}
		for _, comm1 := range knotter.comms1 {
			knotter.formKnotSingle(comm0, comm1)
		}
	}
}

func (knotter *Knotter) doFormKnotsParallel() {
	knotter.doFormKnotsinThread(knotter.comms0)
	knotter.doFormKnotsinThread(knotter.comms1)
}

func (knotter *Knotter) doFormKnotsinThread(comms []interleaving.Communication) {
	for i := 0; i < len(comms); i++ {
		for j := i + 1; j < len(comms); j++ {
			comm0, comm1 := comms[i], comms[j]
			knotter.formKnotSingle(comm0, comm1)
		}
	}
}

func (knotter *Knotter) formKnotSingle(comm0, comm1 interleaving.Communication) {
	knot := interleaving.Knot{comm0, comm1}
	parallel := knot.Type() == interleaving.KnotParallel
	if parallel && comm0.Former().Timestamp > comm1.Former().Timestamp {
		knot = interleaving.Knot{comm1, comm0}
	}
	if knotter.opts.flagSet(FlagWantMessagePassing) {
		if !(comm0.Former().Typ == comm1.Former().Typ &&
			comm0.Latter().Typ == comm1.Latter().Typ &&
			comm0.Former().Typ != comm0.Latter().Typ) {
			return
		}
	}
	if knotter.alreadyHave(knot) {
		return
	}
	if knotter.tooFarComms(comm0, comm1, parallel) {
		return
	}
	if typ := knot.Type(); typ == interleaving.KnotInvalid {
		return
	}
	knotter.knots = append(knotter.knots, knot)
}

func (knotter *Knotter) alreadyHave(knot interleaving.Knot) bool {
	if knotter.opts.Signal == nil {
		return false
	}
	hsh := knot.Hash()
	_, ok := (*knotter.opts.Signal)[hsh]
	return ok
}

func (knotter *Knotter) tooManyNestedComm(comm interleaving.Communication) bool {
	return knotter.innerCommCount[comm] >= knotter.config.threshold
}

func (knotter *Knotter) tooFarComms(comm0, comm1 interleaving.Communication, parallel bool) bool {
	tooFar := func(acc0, acc1 interleaving.Access) bool {
		if acc0.Thread != acc1.Thread {
			panic("wrong")
		}
		tid := acc0.Thread
		windowSize := knotter.windowSize[tid]
		factor := knotter.config.factor
		if windowSize < 2000 {
			factor = 1
		}
		dist := timeDiff(acc0, acc1)
		return dist > uint32(float32(windowSize)*factor)
	}
	if parallel {
		// TODO: At this point, we don't know exactly what the
		// condition of tooFar should be.
		return tooFar(comm0.Former(), comm1.Former()) && tooFar(comm0.Latter(), comm1.Latter())
	} else {
		return tooFar(comm0.Former(), comm1.Latter()) && tooFar(comm1.Latter(), comm0.Former())
	}
}

func timeDiff(acc0, acc1 interleaving.Access) (dist uint32) {
	if acc0.Timestamp > acc1.Timestamp {
		dist = acc0.Timestamp - acc1.Timestamp
	} else {
		dist = acc1.Timestamp - acc0.Timestamp
	}
	return dist
}

func (knotter *Knotter) GetCommunications() []interleaving.Segment {
	return knotter.comms
}

func (knotter *Knotter) GetKnots() []interleaving.Segment {
	return knotter.knots
}

type Scheduler struct {
	// input
	Knots []interleaving.Knot
	// output
	schedPoints []interleaving.Access
}

func (sched *Scheduler) GenerateSchedPoints() ([]interleaving.Access, bool) {
	dag := sched.buildDAG()
	nodes, ok := dag.topologicalSort()
	if !ok {
		return nil, false
	}

	// TODO: This is a current implementation's limitation. QEMU
	// cannot handle schedpoints on the same instruction
	type k struct {
		inst   uint32
		thread uint32
	}
	m := make(map[k]struct{})

	for _, node := range nodes {
		acc := node.(interleaving.Access)
		k0 := k{inst: acc.Inst, thread: uint32(acc.Thread)}
		if _, ok := m[k0]; ok {
			continue
		}
		m[k0] = struct{}{}
		sched.schedPoints = append(sched.schedPoints, interleaving.Access(acc))
	}
	return sched.schedPoints, true
}

func (sched *Scheduler) buildDAG() dag {
	d := newDAG()
	threads := make(map[uint64][]interleaving.Access)
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

const thresholdKnots = 20000
