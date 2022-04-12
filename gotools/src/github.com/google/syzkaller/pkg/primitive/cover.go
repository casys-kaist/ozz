package primitive

type Cover []Segment

func (c Cover) Serialize() []uint32 {
	// A single Knot has 2 Communications, which means has 4
	// Accesses, which means has 28 uint32.
	ret := make([]uint32, 0, len(c)*sizePerKnot)
	for _, segment := range c {
		// XXX: This is ugly, as we accept []Segment as an input but
		// assume it is actually []Knot. This is likely an indication
		// of a wrong design of Segment.
		if knot, ok := segment.(Knot); !ok {
			panic("don't support")
		} else {
			for _, comm := range knot {
				for _, acc := range comm {
					ret = append(ret, acc.Inst,
						acc.Addr,
						acc.Size,
						acc.Typ,
						acc.Timestamp,
						uint32(acc.Thread),
						acc.Context,
					)
				}
			}
		}
	}
	return ret
}

func Deserialize(raw []uint32) Cover {
	c := make(Cover, 0, len(raw)/sizePerKnot)
	for i := 0; i < len(raw); i += sizePerKnot {
		c = append(c, deserializeKnot(raw[i:i+sizePerKnot]))
	}
	return c
}

func deserializeKnot(raw []uint32) Knot {
	knot := Knot{}
	for i := 0; i < 2; i++ {
		comm := Communication{}
		for j := 0; j < 2; j++ {
			from := i*sizePerCommunication + j*sizePerAccess
			to := from + sizePerAccess
			comm[j] = deserializeAccess(raw[from:to])
		}
		knot[i] = comm
	}
	return knot
}

func deserializeAccess(raw []uint32) Access {
	return Access{
		Inst:      raw[0],
		Addr:      raw[1],
		Size:      raw[2],
		Typ:       raw[3],
		Timestamp: raw[4],
		Thread:    uint64(raw[5]),
		Context:   raw[6],
	}
}

const (
	sizePerAccess        = 7
	sizePerCommunication = sizePerAccess * 2
	sizePerKnot          = sizePerCommunication * 2
)
