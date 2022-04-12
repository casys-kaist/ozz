package interleaving

type Signal map[uint64]struct{}
type SerialSignal []uint64

func (i Signal) Serialize() SerialSignal {
	ret := make(SerialSignal, 0, len(i))
	for s := range i {
		ret = append(ret, s)
	}
	return ret
}

func (serial SerialSignal) Deserialize() Signal {
	ret := make(Signal)
	for _, s := range serial {
		ret[s] = struct{}{}
	}
	return ret
}

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

func (i *Signal) DiffRaw(prims []Segment) []Segment {
	diff := []Segment{}
	for _, prim := range prims {
		hsh := prim.Hash()
		if _, ok := (*i)[hsh]; ok {
			continue
		}
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
