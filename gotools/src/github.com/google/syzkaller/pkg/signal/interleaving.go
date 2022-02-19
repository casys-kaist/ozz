package signal

import "github.com/google/syzkaller/pkg/primitive"

type Interleaving map[uint64]struct{}

func (i Interleaving) Diff(i0 Interleaving) Interleaving {
	diff := make(Interleaving)
	for hsh := range i0 {
		if _, ok := i[hsh]; ok {
			continue
		}
		diff[hsh] = struct{}{}
	}
	return diff
}

func (i *Interleaving) DiffMergePrimitive(prims []primitive.Segment) []primitive.Segment {
	diff := []primitive.Segment{}
	for _, prim := range prims {
		hsh := prim.Hash()
		if _, ok := (*i)[hsh]; ok {
			continue
		}
		(*i)[hsh] = struct{}{}
		diff = append(diff, prim)
	}
	return diff
}

func (i *Interleaving) Merge(i0 Interleaving) {
	if i == nil {
		*i = make(Interleaving)
	}
	for hsh := range i0 {
		(*i)[hsh] = struct{}{}
	}
}

func (i Interleaving) Len() int {
	return len(i)
}

func FromPrimitive(prims []primitive.Segment) Interleaving {
	interleaving := make(Interleaving)
	for _, prim := range prims {
		hsh := prim.Hash()
		interleaving[hsh] = struct{}{}
	}
	return interleaving
}
