package primitive

import (
	"fmt"
	"sort"
)

// TODO: Access contains both static information (i.e., Inst, Size,
// Type, Context, ...) and dynamic information (i.e., Addr,
// Thread). Split the struct type into two for them.
type Access struct {
	Inst      uint32
	Addr      uint32
	Size      uint32
	Typ       uint32
	Timestamp uint32
	// TODO: do we need to keep epoch?
	Thread uint64

	Context uint32
}

func (acc Access) String() string {
	return fmt.Sprintf("thread #%d (ctx %x): %x accesses %x (size: %d, type: %d, timestamp: %d)",
		acc.Thread, acc.Context, acc.Inst, acc.Addr, acc.Size, acc.Typ, acc.Timestamp)
}

func (acc Access) Overlapped(acc2 Access) bool {
	min, max := acc.Addr, acc.Addr+acc.Size-1
	return !(acc2.Addr+acc2.Size-1 < min || acc2.Addr > max)
}

type SerialAccess []Access

func SerializeAccess(acc []Access) SerialAccess {
	// NOTE: acc is not sorted when this function is called by
	// FromAcesses. Although SerialAccess will sort them, it is too
	// slow since moving elements need to copy lots of memory
	// objects. To take advantage of the fast path (i.e., idx == n in
	// Add()), we sort acc here and then hand it to serial.Add().
	sort.Slice(acc, func(i, j int) bool { return acc[i].Timestamp < acc[j].Timestamp })
	serial := SerialAccess{}
	for _, acc := range acc {
		serial.Add(acc)
	}
	return serial
}

func (serial SerialAccess) SingleThread() bool {
	for i := 1; i < len(serial); i++ {
		if serial[i].Thread != serial[i-1].Thread {
			return false
		}
	}
	return true
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

func (serial SerialAccess) FindIndex(acc Access) int {
	i := sort.Search(len(serial), func(i int) bool { return serial[i].Timestamp >= acc.Timestamp })
	if i < len(serial) && serial[i].Timestamp == acc.Timestamp {
		return i
		// x is present at data[i]
	} else {
		return -1
	}
}

func Combine(s1, s2 SerialAccess) (s SerialAccess) {
	i1, i2 := 0, 0
	for i1 < len(s1) && i2 < len(s2) {
		acc1, acc2 := s1[i1], s2[i2]
		if acc1.Timestamp < acc2.Timestamp {
			s.Add(acc1)
			i1++
		} else {
			s.Add(acc2)
			i2++
		}
	}
	for ; i1 < len(s1); i1++ {
		s.Add(s1[i1])
	}
	for ; i2 < len(s2); i2++ {
		s.Add(s2[i2])
	}
	return
}

// TODO: This function is somehow broken and must be removed. See
// scheduler.addPoint() and scheduler.makePoint() in prog/schedule.go
func (serial SerialAccess) FindForeachThread(inst uint32, max int) SerialAccess {
	// Find at most max Accesses for each thread that are executed at inst
	chk := make(map[uint64]int)
	res := SerialAccess{}
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

const (
	TypeStore = iota
	TypeLoad
)
