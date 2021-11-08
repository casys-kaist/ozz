package signal

import (
	"container/heap"
	"fmt"
	"sort"
)

type key struct{ from, to uint32 }

func makeKey(from, to uint32) key { return key{from: from, to: to} }

// ReadFrom represents read-from coverage for two instructions. For
// given two instructions inst1 and inst2, if ok is true where _, ok
// := rf[inst1][inst2], inst1 affects inst2 which means inst2 reads
// from inst1.
type ReadFrom map[key]struct{}

func NewReadFrom() ReadFrom { return make(map[key]struct{}) }

func (rf ReadFrom) containKey(k key) bool {
	_, ok := rf[k]
	return ok
}

func (rf ReadFrom) Contain(from, to uint32) bool {
	k := makeKey(from, to)
	return rf.containKey(k)
}

func (rf ReadFrom) Empty() bool {
	return len(rf) == 0
}

func (rf ReadFrom) addKey(k key) {
	rf[k] = struct{}{}
}

func (rf ReadFrom) Add(from, to uint32) {
	k := makeKey(from, to)
	rf.addKey(k)
}

func (rf ReadFrom) Copy() ReadFrom {
	new := ReadFrom{}
	for k := range rf {
		new.addKey(k)
	}
	return new
}

func (rf ReadFrom) Merge(rf1 ReadFrom) {
	if rf1.Empty() {
		return
	}
	for k := range rf1 {
		rf.addKey(k)
	}
}

func (rf ReadFrom) Diff(rf1 ReadFrom) ReadFrom {
	res := ReadFrom{}
	for k := range rf1 {
		if !rf.containKey(k) {
			res.addKey(k)
		}
	}
	return res
}

func (rf ReadFrom) Len() int {
	return len(rf)
}

func (rf ReadFrom) Flatting() []uint32 {
	r := []uint32{}
	c := make(map[uint32]struct{})
	for k := range rf {
		if _, ok := c[k.from]; !ok {
			c[k.from] = struct{}{}
			r = append(r, k.from)
		}
		if _, ok := c[k.to]; !ok {
			c[k.to] = struct{}{}
			r = append(r, k.to)
		}
	}
	return r
}

type Order uint32

const (
	Before = iota
	Parallel
	After
)

func FromEpoch(i1, i2 uint64) Order {
	if i1 < i2 {
		return Before
	} else if i1 == i2 {
		return Parallel
	} else {
		return After
	}
}

// Build ReadFrom interactions from two sequences of accesses, acc1
// and acc2
// TODO: signal uses prio describing priority of each element. I
// have no idea that we need it too.
func FromAccesses(acc1, acc2 []Access, order Order) (ReadFrom, SerialAccess) {
	if order == After {
		// if acc1 happened after acc2, nothing from acc2 could be
		// affected by acc1.
		return nil, nil
	}

	const (
		store = 0
		load  = 1
	)

	sort.Slice(acc1, func(i, j int) bool { return acc1[i].timestamp < acc1[j].timestamp })
	sort.Slice(acc2, func(i, j int) bool { return acc2[i].timestamp < acc2[j].timestamp })

	rf := NewReadFrom()
	used := []Access{}
	m := make(map[uint32]*Access)

	visitAcc := func(acc *Access) {
		if acc0, ok := m[acc.addr]; ok && (acc0.thread != acc.thread || order != Parallel) {
			rf.Add(acc0.inst, acc.inst)
			used = append(used, *acc0, *acc)
		}
		if acc.typ == store {
			m[acc.addr] = acc
		}
	}

	var i1, i2 int
	for i1, i2 = 0, 0; i1 < len(acc1) && i2 < len(acc2); {
		var acc *Access
		if acc1[i1].timestamp < acc2[i2].timestamp {
			acc = &acc1[i1]
			i1++
		} else {
			acc = &acc2[i2]
			i2++
		}
		visitAcc(acc)
	}

	for ; i1 < len(acc1); i1++ {
		visitAcc(&acc1[i1])
	}
	for ; i2 < len(acc2); i2++ {
		visitAcc(&acc2[i2])
	}
	serial := serializeAccess(used)
	return rf, serial
}

type Access struct {
	inst      uint32
	addr      uint32
	size      uint32
	typ       uint32
	timestamp uint32
	// TODO: do we need to keep epoch?
	thread uint64
}

func NewAccess(inst, addr, size, typ, timestamp uint32, thread, epoch uint64) Access {
	return Access{inst, addr, size, typ, timestamp, thread}
}

func (acc Access) String() string {
	return fmt.Sprintf("thread #%d: %x accesses %x (size: %d, type: %d, timestamp: %d)",
		acc.thread, acc.inst, acc.addr, acc.size, acc.typ, acc.timestamp)
}

func (acc Access) ExecutedBy(thread uint64) bool {
	return acc.thread == thread
}

// TODO: expose each fields
func (acc Access) Thread() uint64 {
	return acc.thread
}

func (acc Access) Inst() uint32 {
	return acc.inst
}

func (acc Access) Owned(inst uint64, thread uint64) bool {
	// TODO: possibly temporary. used by only scheduler.findAccess()
	// (i.e., prog/schedule.go)
	return acc.inst == uint32(inst) && acc.thread == thread
}

type SerialAccess []Access

func NewSerialAccess() SerialAccess {
	serial := SerialAccess{}
	heap.Init(&serial)
	return serial
}

func (serial SerialAccess) Len() int {
	return len(serial)
}

func (serial SerialAccess) Less(i, j int) bool {
	return serial[i].timestamp < serial[j].timestamp
}

func (serial SerialAccess) Swap(i, j int) {
	serial[i], serial[j] = serial[j], serial[i]
}

func (serial *SerialAccess) Push(element interface{}) {
	*serial = append(*serial, element.(Access))
}

func (serial *SerialAccess) Pop() interface{} {
	old := *serial
	n := len(old)
	access := old[n-1]
	*serial = old[0 : n-1]
	return access
}

func serializeAccess(acc []Access) SerialAccess {
	serial := NewSerialAccess()
	for _, acc := range acc {
		serial.Add(acc)
	}
	return serial
}

func (serial *SerialAccess) Add(acc Access) {
	serial.Push(acc)
}

func (serial SerialAccess) Find(inst uint32, max int) SerialAccess {
	// Find at most max Accesses for each thread that are executed at inst
	chk := make(map[uint64]int)
	res := NewSerialAccess()
	for _, acc := range serial {
		if cnt := chk[acc.thread]; acc.inst == inst && cnt < max {
			res.Add(acc)
			chk[acc.thread]++
		}
		if len(res) == max*2 {
			// TODO: Razzer's mechanism. We execute at most two
			// syscalls in parallel (i.e., the maximum length of res
			// is max*2).
			break
		}
	}
	return res
}
