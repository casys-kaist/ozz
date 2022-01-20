package primitive

import (
	"fmt"
	"sort"
)

type Access struct {
	Inst      uint32
	Addr      uint32
	Size      uint32
	Typ       uint32
	Timestamp uint32
	// TODO: do we need to keep epoch?
	Thread uint64
}

func (acc Access) String() string {
	return fmt.Sprintf("thread #%d: %x accesses %x (size: %d, type: %d, timestamp: %d)",
		acc.Thread, acc.Inst, acc.Addr, acc.Size, acc.Typ, acc.Timestamp)
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
