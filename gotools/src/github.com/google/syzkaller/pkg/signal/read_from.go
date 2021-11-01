package signal

import (
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

	rf := NewReadFrom()
	used := []Access{}

	for _, a1 := range acc1 {
		if a1.typ == load {
			// only store operations can affect acc2
			continue
		}
		for _, a2 := range acc2 {
			// we don't care the type of a2 since we track store-load
			// and store-store relations
			if a1.addr>>3 != a2.addr>>3 {
				// TODO: check precisely using .size. testdata/gen.py
				// is also needed to be fixed.
				continue
			}
			if order == Before || a1.timestamp < a2.timestamp {
				// a1 is store which executed before a2
				rf.Add(a1.inst, a2.inst)
				used = append(used, a1, a2)
			}
		}
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
}

func NewAccess(inst, addr, size, typ, timestamp uint32) Access {
	return Access{inst: inst, addr: addr, size: size, typ: typ, timestamp: timestamp}
}

func (acc Access) String() string {
	return fmt.Sprintf("%x accesses %x (size: %d, type: %d, timestamp: %d)",
		acc.inst, acc.addr, acc.size, acc.typ, acc.timestamp)
}

type SerialAccess []uint32

func serializeAccess(acc []Access) SerialAccess {
	serial := SerialAccess{}
	sort.Slice(acc, func(i, j int) bool { return acc[i].timestamp < acc[j].timestamp })
	for _, acc := range acc {
		serial.Add(acc)
	}
	return serial
}

func (serial SerialAccess) Add(acc Access) {
	serial = append(serial, acc.inst)
}
