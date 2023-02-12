package interleaving

import (
	"bytes"
	"fmt"
	"strconv"
)

type Signal map[uint64]struct{}

func (i Signal) Copy() Signal {
	c := make(Signal, len(i))
	for e := range i {
		c[e] = struct{}{}
	}
	return c
}

func (i *Signal) Split(n int) Signal {
	if i.Empty() {
		return nil
	}
	c := make(Signal, n)
	for e := range *i {
		delete(*i, e)
		c[e] = struct{}{}
		n--
		if n == 0 {
			break
		}
	}
	if len(*i) == 0 {
		*i = nil
	}
	return c
}

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

func (i *Signal) Merge(i1 Signal) {
	i0 := *i
	if i0 == nil {
		i0 = make(Signal, len(i1))
		*i = i0
	}
	for hsh := range i1 {
		(*i)[hsh] = struct{}{}
	}
}

func (i Signal) Len() int {
	return len(i)
}

func (i Signal) ToHex() (ret []byte) {
	for k := range i {
		hex := fmt.Sprintf("%x\n", k)
		ret = append(ret, []byte(hex)...)
	}
	return
}

func (i *Signal) FromHex(ret []byte) {
	i0 := *i
	if i0 == nil {
		i0 = make(Signal)
		*i = i0
	}
	raws := bytes.Fields(ret)
	for _, raw := range raws {
		sig, err := strconv.ParseUint(string(raw), 16, 64)
		if err != nil {
			panic(err)
		}
		(*i)[sig] = struct{}{}
	}
}

func FromCoverToSignal(c Cover) Signal {
	interleaving := make(Signal)
	for _, c := range c {
		hsh := c.Hash()
		interleaving[hsh] = struct{}{}
	}
	return interleaving
}
