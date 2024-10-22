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
	// NOTE: Assumption: there are only two threads.
	b := make([]byte, 32)
	w := writer{b: b}
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			w.write(knot[i][j].Inst)
			var normalized uint32
			if knot[i][j].Timestamp > knot[1-i][1].Timestamp {
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

type writer struct {
	b []byte
}

func (w *writer) write(v uint32) {
	binary.LittleEndian.PutUint32(w.b, v)
	w.b = w.b[4:]
}
