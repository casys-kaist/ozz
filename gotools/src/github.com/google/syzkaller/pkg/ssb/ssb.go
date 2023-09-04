package ssb

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
)

type tableEntry struct {
	inst  uint64
	value int
}

type FlushVector struct {
	table  []tableEntry
	vector []uint32
}

func (vec FlushVector) Valid() bool {
	return len(vec.table) == 0 || len(vec.vector) == 0
}

func (vec *FlushVector) AddTableEntry(inst uint64, value int) {
	entry := tableEntry{inst: inst, value: value}
	vec.table = append(vec.table, entry)
}

func (vec *FlushVector) AddVectorEntry(v uint32) {
	vec.vector = append(vec.vector, v)
}

func (vec FlushVector) SerializeVector() []uint32 {
	return vec.vector
}

func (vec FlushVector) SerializeTable() []uint64 {
	r := []uint64{}
	for _, e := range vec.table {
		r = append(r, e.inst, uint64(e.value))
	}
	return r
}

func GenerateFlushVector(r *rand.Rand, cand interleaving.Candidate) FlushVector {
	doRandom := func() bool {
		if r == nil {
			return false
		}
		// 0.5%
		return r.Intn(1000) < 5
	}
	if !cand.Invalid() && !doRandom() {
		return generateFlushVectorForCandidate(cand)
	} else {
		// Return a random flush vector
		return generateRandomFlushVector(r)
	}
}

func generateFlushVectorForCandidate(cand interleaving.Candidate) FlushVector {
	uext := func(v uint32) uint64 {
		return uint64(v) | 0xffffffff00000000
	}
	table := []tableEntry{}
	ht := make(map[uint32]struct{})
	_add_entry := func(i uint32, v int) {
		if _, ok := ht[i]; ok {
			return
		}
		ht[i] = struct{}{}
		table = append(table, tableEntry{inst: uext(i), value: v})
	}
	for _, acc := range cand.DelayingInst {
		_add_entry(acc.Inst, 0)
	}
	// TODO: Reuse generated sched point
	// Sched point (= first acc of crit comm) should be 1
	schedPoint := cand.CriticalComm.Former()
	_add_entry(schedPoint.Inst, 1)
	return FlushVector{table: table}
}

func generateRandomFlushVector(r *rand.Rand) FlushVector {
	const (
		MAX_LEGNTH = 10
		MAX_VALUE  = 1
	)
	// Random FlushVector uses the vector interface
	len := (r.Uint32()%(MAX_LEGNTH-1) + 2)
	vector := make([]uint32, len)
	for i := uint32(0); i < len; i++ {
		vector[i] = r.Uint32() % (MAX_VALUE + 1)
	}
	return FlushVector{vector: vector}
}
