package primitive

import (
	"github.com/mitchellh/hashstructure"
)

type Segment interface {
	Hash() uint64
}

func (comm Communication) Hash() uint64 {
	return hash(comm)
}

func (knot Knot) Hash() uint64 {
	return hash(knot)
}

func hash(v interface{}) uint64 {
	hash, err := hashstructure.Hash(v, nil)
	if err != nil {
		panic(err)
	}
	return hash
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
