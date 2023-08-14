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
	for _, acc := range cand.DelayingInst {
		table = append(table, tableEntry{inst: uext(acc.Inst), value: 0})
	}
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
