package primitive

import (
	"encoding/binary"
	"hash/fnv"
)

type Segment interface {
	Hash() uint64
}

func (comm Communication) Hash() uint64 {
	b := make([]byte, 24)
	w := writer{b: b}
	for i := 0; i < 2; i++ {
		w.write(comm[i].Inst)
		w.write(uint32(comm[i].Thread))
		w.write(uint32(comm[i].Timestamp))
	}
	return hash(b)
}

func (knot Knot) Hash() uint64 {
	// NOTE: Assumption: the knot type is not Invalid or Parallel, and
	// there are only two threads. TODO: extend the implmentation if
	// needed.
	b := make([]byte, 48)
	w := writer{b: b}
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			w.write(knot[i][j].Inst)
			w.write(uint32(knot[i][j].Thread))
			w.write(knot[i][j].Timestamp)
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
