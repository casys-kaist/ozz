package interleaving

type Signal map[uint64]struct{}

func (i Signal) Empty() bool {
	return len(i) == 0
}

func (i Signal) Diff(i0 Signal) Signal {
	diff := make(Signal)
	for hsh := range i0 {
		if _, ok := i[hsh]; ok {
			continue
		}
		diff[hsh] = struct{}{}
	}
	return diff
}

func (i *Signal) DiffMergePrimitive(prims []Segment) []Segment {
	diff := []Segment{}
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

func (i *Signal) Merge(i0 Signal) {
	if i == nil {
		*i = make(Signal)
	}
	for hsh := range i0 {
		(*i)[hsh] = struct{}{}
	}
}

func (i Signal) Len() int {
	return len(i)
}

func FromCoverToSignal(c Cover) Signal {
	interleaving := make(Signal)
	for _, c := range c {
		hsh := c.Hash()
		interleaving[hsh] = struct{}{}
	}
	return interleaving
}
