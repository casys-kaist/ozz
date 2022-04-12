package interleaving

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
	} else if knot.invalidContext() {
		return KnotInvalid
	} else if comm0.HappenBefore(comm1) || comm1.HappenBefore(comm0) {
		return KnotSeparated
	} else {
		return KnotOverlapped
	}
}

func (knot Knot) invalidContext() bool {
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			acc0 := knot[0][i]
			acc1 := knot[1][j]
			if acc0.Thread != acc1.Thread {
				continue
			}
			if ctx0, ctx1 := acc0.Context, acc1.Context; ctx0 == ctx1 || ctx0 == CommonPath || ctx1 == CommonPath {
				// Two accesses in a same thread can exist if 1) they
				// are in the same context, or 2) one resides on the
				// common path.
				return false
			} else {
				return true
			}
		}
	}
	return false
}

const CommonPath = 0xff
