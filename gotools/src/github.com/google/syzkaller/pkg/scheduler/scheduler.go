package scheduler

import (
	"github.com/google/syzkaller/pkg/primitive"
)

// NOTE: Communicadtion[0] must/will happen before Communication[1]
type Communication [2]primitive.Access

func (comm Communication) Same(comm0 Communication) bool {
	return comm[0].Inst == comm0[0].Inst && comm[1].Inst == comm0[1].Inst
}

type Knot [2]Communication

func (knot Knot) Same(knot0 Knot) bool {
	return knot[0].Same(knot0[0]) && knot[1].Same(knot0[1])
}

type knotter struct {
	accesses []primitive.SerialAccess
	knots    []Knot
}

func ExcavateKnots(accesses []primitive.SerialAccess) []Knot {
	knotter := knotter{
		accesses: accesses,
	}
	knotter.fastenKnots()
	return knotter.knots
}

func (knotter *knotter) fastenKnots() {
	mp := make(map[uint32][]primitive.Access)
	for _, accs := range knotter.accesses {
		for _, acc := range accs {
			addr := acc.Addr & ^uint32(7)
			mp[addr] = append(mp[addr], acc)
		}
	}

	comms := []Communication{}
	for _, accs := range mp {
		comms = append(comms, formCommunication(accs)...)
	}
	knotter.formKnots(comms)
}

func formCommunication(accesses []primitive.Access) []Communication {
	comms := []Communication{}
	for i := 0; i < len(accesses); i++ {
		for j := i + 1; j < len(accesses); j++ {
			acc1, acc2 := accesses[i], accesses[j]
			if acc1.Thread == acc2.Thread {
				continue
			}

			if acc1.Typ == primitive.TypeLoad && acc2.Typ == primitive.TypeLoad {
				continue
			}

			if !acc1.Overlapped(acc2) {
				continue
			}

			// We are generating all possible knots so append both
			// Communications
			comms = append(comms, Communication{acc1, acc2}, Communication{acc2, acc1})
		}
	}
	return comms
}

func (knotter *knotter) formKnots(comms []Communication) {
	for i := 0; i < len(comms); i++ {
		for j := i + 1; j < len(comms); j++ {
			comm1, comm2 := comms[i], comms[j]
			if comm1[0].Timestamp > comm2[0].Timestamp {
				comm1, comm2 = comm2, comm1
			}
			knot := Knot{comm1, comm2}
			if typ := knot.Type(); typ == KnotParallel || typ == KnotInvalid {
				continue
			}
			knotter.knots = append(knotter.knots, knot)
		}
	}
}

func (knot Knot) Type() KnotType {
	parallel := true
	for i := 0; i < 2; i++ {
		if knot[0][i].Thread != knot[1][i].Thread {
			parallel = false
			break
		}
	}
	if parallel {
		return KnotParallel
	}

	comm0, comm1 := knot[0], knot[1]

	// NOTE: Assumption: Accesses's timestamps in SerialAccess have
	// the same order as the program order
	conflict := func(comm0, comm1 Communication) bool {
		// Let's check comm1 conflicts to comm0
		return comm0[1].Timestamp <= comm1[0].Timestamp && comm1[1].Timestamp <= comm0[0].Timestamp
	}

	happenBefore := func(comm0, comm1 Communication) bool {
		return comm0[1].Timestamp < comm1[0].Timestamp &&
			comm0[0].Timestamp < comm1[1].Timestamp
	}

	if conflict(comm0, comm1) || conflict(comm1, comm0) {
		// Invalid program orders
		return KnotInvalid
	}

	if happenBefore(comm0, comm1) || happenBefore(comm1, comm0) {
		return KnotSeparated
	} else {
		return KnotOverlapped
	}
}

type KnotType int

const (
	KnotInvalid KnotType = iota
	KnotParallel
	KnotOverlapped
	KnotSeparated
)
