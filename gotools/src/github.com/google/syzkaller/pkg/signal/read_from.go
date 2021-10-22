package signal

// ReadFrom represents read-from coverage for two instructions. For
// given two instructions inst1 and inst2, if ok is true where _, ok
// := rf[inst1][inst2], inst1 affects inst2 which means inst2 reads
// from inst1.
type ReadFrom map[uint32]map[uint32]struct{}

func (rf ReadFrom) Empty() bool {
	if len(rf) == 0 {
		return true
	}
	// XXX: This loop wastes computing power. Do not retains an empty
	// map in the middle if possible.
	for _, v := range rf {
		if len(v) != 0 {
			return false
		}
	}
	return true
}

type Order uint32

const (
	Before = Order(1)
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
func FromAccesses(acc1, acc2 []Access, order Order) ReadFrom {
	if order == After {
		// if acc1 happened after acc2, nothing from acc2 could be
		// affected by acc1.
		return nil
	}

	const (
		store = 0
		load  = 1
	)

	res := make(map[uint32]map[uint32]struct{})
	add := func(inst1, inst2 uint32) {
		if _, ok := res[inst1]; !ok {
			res[inst1] = make(map[uint32]struct{})
		}
		res[inst1][inst2] = struct{}{}
	}

	for _, a1 := range acc1 {
		if a1.typ != load {
			// only store operations can affect acc2
			continue
		}
		for _, a2 := range acc2 {
			// we don't care the type of a2 since we track store-load
			// and store-store relations
			if a1.addr>>3 != a2.addr>>3 {
				// TODO: check precisely using .size
				continue
			}
			if order == Before || a1.timestamp < a2.timestamp {
				// a1 is store which executed before a2
				add(a1.inst, a2.inst)
			}
		}
	}

	return res
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
