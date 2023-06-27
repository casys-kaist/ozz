package ssb

import "math/rand"

type FlushVector []uint32

func GenerateFlushVector(r *rand.Rand) FlushVector {
	const (
		max_length = 10
		max_value  = 5
	)
	len := r.Uint32() % max_length
	vec := make(FlushVector, len)
	for i := uint32(0); i < len; i++ {
		vec[i] = r.Uint32() % max_value
	}
	return vec
}

func (vec FlushVector) Len() int {
	return len(vec)
}
