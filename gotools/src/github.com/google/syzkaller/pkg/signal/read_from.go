package signal

import (
	"fmt"
	"sort"
)

type key struct{ from, to uint32 }

func (k key) String() string {
	return fmt.Sprintf("%x -> %x", k.from, k.to)
}

func makeKey(from, to uint32) key { return key{from: from, to: to} }

// ReadFrom represents read-from coverage for two instructions. For
// given two instructions inst1 and inst2, if ok is true where _, ok
// := rf[inst1][inst2], inst1 affects inst2 which means inst2 reads
// from inst1.
type ReadFrom map[key]struct{}

func NewReadFrom() ReadFrom { return make(map[key]struct{}) }

func (rf ReadFrom) Serialize() SerialReadFrom {
	res := SerialReadFrom{}
	for k := range rf {
		res.From = append(res.From, k.from)
		res.To = append(res.To, k.to)
	}
	return res
}

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

func (rf *ReadFrom) Split(n int) ReadFrom {
	if rf.Empty() {
		return nil
	}
	ret := NewReadFrom()
	for k := range *rf {
		delete(*rf, k)
		ret.addKey(k)
		n--
		if n == 0 {
			break
		}
	}
	if len(*rf) == 0 {
		*rf = nil
	}
	return ret
}

type SerialReadFrom struct {
	From []uint32
	To   []uint32
}

func (s SerialReadFrom) Len() int {
	return len(s.From)
}

func (s SerialReadFrom) Deserialize() ReadFrom {
	res := ReadFrom{}
	l := len(s.From)
	for i := 0; i < l; i++ {
		res.Add(s.From[i], s.To[i])
	}
	return res
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

	sort.Slice(acc1, func(i, j int) bool { return acc1[i].Timestamp < acc1[j].Timestamp })
	sort.Slice(acc2, func(i, j int) bool { return acc2[i].Timestamp < acc2[j].Timestamp })

	rf := NewReadFrom()
	used := []Access{}
	m := make(map[uint32]*Access)
	t := make(map[*Access]int)

	samecall := func(acc0, acc1 *Access) bool {
		if acc0.Thread != acc1.Thread {
			return true
		}
		if t[acc0] != t[acc1] {
			return true
		}
		return false
	}

	visitAcc := func(acc *Access) {
		if acc0, ok := m[acc.Addr>>3]; ok && samecall(acc0, acc) {
			rf.Add(acc0.Inst, acc.Inst)
			used = append(used, *acc0, *acc)
		}
		if acc.Typ == store {
			m[acc.Addr>>3] = acc
		}
	}

	var i1, i2 int
	for i1, i2 = 0, 0; i1 < len(acc1) && i2 < len(acc2); {
		var acc *Access
		if acc1[i1].Timestamp < acc2[i2].Timestamp {
			acc = &acc1[i1]
			t[acc] = 1
			i1++
		} else {
			acc = &acc2[i2]
			t[acc] = 2
			i2++
		}
		visitAcc(acc)
	}

	for ; i1 < len(acc1); i1++ {
		t[&acc1[i1]] = 1
		visitAcc(&acc1[i1])
	}
	for ; i2 < len(acc2); i2++ {
		t[&acc2[i2]] = 2
		visitAcc(&acc2[i2])
	}
	serial := serializeAccess(used)
	return rf, serial
}

type Access struct {
	Inst      uint32
	Addr      uint32
	Size      uint32
	Typ       uint32
	Timestamp uint32
	// TODO: do we need to keep epoch?
	Thread uint64
}

func NewAccess(inst, addr, size, typ, timestamp uint32, thread, epoch uint64) Access {
	return Access{inst, addr, size, typ, timestamp, thread}
}

func (acc Access) String() string {
	return fmt.Sprintf("thread #%d: %x accesses %x (size: %d, type: %d, timestamp: %d)",
		acc.Thread, acc.Inst, acc.Addr, acc.Size, acc.Typ, acc.Timestamp)
}

func (acc Access) ExecutedBy(thread uint64) bool {
	return acc.Thread == thread
}

func (acc Access) Owned(inst uint64, thread uint64) bool {
	// TODO: possibly temporary. used by only scheduler.findAccess()
	// (i.e., prog/schedule.go)
	return acc.Inst == uint32(inst) && acc.Thread == thread
}

type SerialAccess []Access

func NewSerialAccess() SerialAccess {
	return SerialAccess{}
}

func serializeAccess(acc []Access) SerialAccess {
	// NOTE: acc is not sorted when this function is called by
	// FromAcesses. Although SerialAccess will sort them, it is too
	// slow since moving elements need to copy lots of memory
	// objects. To take advantage of the fast path (i.e., idx == n in
	// Add()), we sort acc here and then hand it to serial.Add().
	sort.Slice(acc, func(i, j int) bool { return acc[i].Timestamp < acc[j].Timestamp })
	serial := NewSerialAccess()
	for _, acc := range acc {
		serial.Add(acc)
	}
	return serial
}

func (serial *SerialAccess) Add(acc Access) {
	n := len(*serial)
	idx := sort.Search(n, func(i int) bool {
		return (*serial)[i].Timestamp >= acc.Timestamp
	})
	if idx == n {
		*serial = append(*serial, acc)
	} else {
		*serial = append((*serial)[:idx+1], (*serial)[idx:]...)
		(*serial)[idx] = acc
	}
}

func (serial SerialAccess) FindForeachThread(inst uint32, max int) SerialAccess {
	// Find at most max Accesses for each thread that are executed at inst
	chk := make(map[uint64]int)
	res := NewSerialAccess()
	for _, acc := range serial {
		if cnt := chk[acc.Thread]; acc.Inst == inst && cnt < max {
			res.Add(acc)
			chk[acc.Thread]++
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

func (serial SerialAccess) FindIndex(acc Access) int {
	for i, acc0 := range serial {
		if acc == acc0 {
			return i
		}
	}
	return -1
}
