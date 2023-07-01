package ssb

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
)

type FlushVector []uint32

func (vec FlushVector) Len() int {
	return len(vec)
}

func GenerateFlushVector(r *rand.Rand, hints []interleaving.Segment) FlushVector {
	doRandom := func() bool {
		if r == nil {
			return false
		}
		// 5%
		return r.Intn(100) < 5
	}
	if len(hints) == 1 && !doRandom() {
		h := hints[0]
		vec, ok := generateFlushVectorForOneHint(r, h)
		if ok {
			return vec
		}
	}
	// Return a random flush vector
	return generateRandomFlushVector(r)
}

func generateRandomFlushVector(r *rand.Rand) FlushVector {
	const (
		MAX_LEGNTH = 10
		MAX_VALUE  = 1
	)
	len := (r.Uint32()%(MAX_LEGNTH-1) + 2)
	vec := make(FlushVector, len)
	for i := uint32(0); i < len; i++ {
		vec[i] = r.Uint32() % (MAX_VALUE + 1)
	}
	return vec
}

func generateFlushVectorForOneHint(r *rand.Rand, h interleaving.Segment) (FlushVector, bool) {
	k, ok := h.(interleaving.Knot)
	if !ok {
		panic("wrong")
	}
	// TODO: need a better solution
	cands := generatePossibleFlushVectors()
	for _, cand := range cands {
		if isReorderingKnot(k, cand) {
			return cand, true
		}
	}
	return FlushVector{}, false
}

var generated []FlushVector = nil

func generatePossibleFlushVectors() (rt []FlushVector) {
	const MAX_LENGTH = 4
	if len(generated) == 0 {
		generated = []FlushVector{}
		for i := 2; i < MAX_LENGTH; i++ {
			generated = append(generated,
				__generatePossibleFlushVectors(i)...,
			)
		}
	}
	return generated
}

// Generate all possible FlushVectors of length n. This is probably
// slow, but we may not need to take care about it since n is small.
// Ref: https://www.sobyte.net/post/2022-01/go-slice/
func __generatePossibleFlushVectors(n int) (rt []FlushVector) {
	if n <= 1 {
		return
	}
	for r := 1; r < n; r++ {
		indices := make([]int, r)
		for i := range indices {
			indices[i] = i
		}

		vec := make(FlushVector, n)
		for _, el := range indices {
			vec[el] = 1
		}
		rt = append(rt, vec)

		for {
			i := r - 1
			for ; i >= 0 && indices[i] == i+n-r; i -= 1 {
			}

			if i < 0 {
				break
			}

			indices[i] += 1
			for j := i + 1; j < r; j += 1 {
				indices[j] = indices[j-1] + 1
			}

			vec := make(FlushVector, n)
			for j := 0; j < len(indices); j += 1 {
				vec[indices[j]] = 1
			}
			rt = append(rt, vec)
		}
	}
	return
}

func isReorderingKnot(knot interleaving.Knot, vec FlushVector) bool {
	// NB: Should be compatible with the kssb implementation in the
	// kernel
	if knot.Type() != interleaving.KnotParallel {
		panic("wrong")
	}
	if knot[0].Former().Typ != interleaving.TypeStore {
		panic("wrong")
	}
	const (
		GOLDEN_RATIO = 0x61C8864680B583EB
		BITS         = 64
	)
	idx := [2][2]int{}
	for i, comm := range knot {
		for j, acc := range comm {
			inst64 := 0xffffffff00000000 | uint64(acc.Inst)
			hsh := (inst64 * GOLDEN_RATIO) >> (64 - BITS)
			idx[i][j] = int(hsh % uint64(len(vec)))
		}
	}

	if knot[1].Former().Typ == interleaving.TypeStore {
		// Message passing. Want:
		// - knot[0][0] --> 0
		// - knot[1][0] --> 1
		return vec[idx[0][0]] == 0 && vec[idx[1][0]] == 1
	} else {
		// OOTA. Want:
		// - knot[0][0] --> 0
		// - knot[1][1] --> 0
		return vec[idx[0][0]] == 0 && vec[idx[1][1]] == 0
	}
}
