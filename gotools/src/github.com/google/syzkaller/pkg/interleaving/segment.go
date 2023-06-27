package interleaving

import (
	"encoding/binary"
	"hash/fnv"
)

type Segment interface {
	Hash() uint64
}

func (comm Communication) Hash() uint64 {
	b := make([]byte, 16)
	w := writer{b: b}
	for i := 0; i < 2; i++ {
		w.write(comm[i].Inst)
		w.write(uint32(i))
	}
	return hash(b)
}

func (knot Knot) Hash() uint64 {
	// NOTE: Assumption: the knot type is not Invalid or Parallel, and
	// there are only two threads. TODO: extend the implmentation if
	// needed.
	b := make([]byte, 32)
	w := writer{b: b}
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			w.write(knot[i][j].Inst)
			var normalized uint32
			if knot[i][j].Timestamp > knot[1-i][1-j].Timestamp {
				normalized = 1
			}
			w.write(normalized)
		}
	}

	return hash(b)
}

func hash(b []byte) uint64 {
	hash := fnv.New64a()
	hash.Write(b)
	return hash.Sum64()
}

func Intersect(s1, s2 []Segment) []Segment {
	i := []Segment{}
	hshtbl := make(map[uint64]struct{})
	for _, s := range s1 {
		hshtbl[s.Hash()] = struct{}{}
	}
	for _, s := range s2 {
		if _, ok := hshtbl[s.Hash()]; ok {
			i = append(i, s)
		}
	}
	return i
}

type writer struct {
	b []byte
}

func (w *writer) write(v uint32) {
	binary.LittleEndian.PutUint32(w.b, v)
	w.b = w.b[4:]
}
