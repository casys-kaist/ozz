package primitive

type Knot [2]Communication

func (knot Knot) Same(knot0 Knot) bool {
	return (knot[0].Same(knot0[0]) && knot[1].Same(knot0[1])) ||
		(knot[0].Same(knot0[1]) && knot[1].Same(knot0[0]))
}

type KnotType int

const (
	KnotInvalid KnotType = iota
	KnotParallel
	KnotOverlapped
	KnotSeparated
)

func (knot Knot) Type() KnotType {
	comm0, comm1 := knot[0], knot[1]
	if comm0.Parallel(comm1) {
		return KnotParallel
	} else if comm0.Conflict(comm1) || comm1.Conflict(comm0) {
		// Invalid program orders
		return KnotInvalid
	} else if comm0.HappenBefore(comm1) || comm1.HappenBefore(comm0) {
		return KnotSeparated
	} else {
		return KnotOverlapped
	}
}
