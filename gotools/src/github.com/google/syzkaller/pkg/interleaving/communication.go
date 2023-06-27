package interleaving

// NOTE: Communicadtion[0] must/will happen before Communication[1]
// NOTE: Assumption: Accesses's timestamps in SerialAccess have
// the same order as the program order
type Communication [2]Access

func (inner Communication) Imply(outer Communication) bool {
	// If inner --> outer
	// outer[0]
	// inner[0]
	//             inner[1]
	//             outer[1]
	chk := func(acc1, acc2 Access) bool {
		return acc1.Context == CommonPath ||
			acc2.Context == CommonPath ||
			acc1.Context == acc2.Context
	}
	if !chk(inner.Former(), outer.Former()) || !chk(inner.Latter(), outer.Latter()) {
		return false
	}
	return inner.Former().Timestamp >= outer.Former().Timestamp &&
		inner.Latter().Timestamp <= outer.Latter().Timestamp
}

func (comm *Communication) Former() Access {
	return comm[0]
}

func (comm *Communication) Latter() Access {
	return comm[1]
}

func (comm Communication) Same(comm0 Communication) bool {
	return comm[0].Inst == comm0[0].Inst && comm[1].Inst == comm0[1].Inst
}

func (comm0 Communication) Conflict(comm1 Communication) bool {
	if comm0.Parallel(comm1) {
		// Parallell comms cannot be conflict
		return false
	}
	return comm0[1].Timestamp <= comm1[0].Timestamp && comm1[1].Timestamp <= comm0[0].Timestamp ||
		comm1[1].Timestamp <= comm0[0].Timestamp && comm0[1].Timestamp <= comm1[0].Timestamp
}

func (comm0 Communication) Parallel(comm1 Communication) bool {
	for i := 0; i < 2; i++ {
		if comm0[i].Thread != comm1[i].Thread {
			return false
		}
	}
	return true
}

func (comm0 Communication) HappenBefore(comm1 Communication) bool {
	return comm0[1].Timestamp < comm1[0].Timestamp &&
		comm0[0].Timestamp < comm1[1].Timestamp
}
