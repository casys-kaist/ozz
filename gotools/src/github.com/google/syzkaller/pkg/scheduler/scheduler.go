package scheduler

import (
	"github.com/google/syzkaller/pkg/primitive"
)

// NOTE: Communicadtion[0] must/will happen before Communication[1]
// NOTE: Assumption: Accesses's timestamps in SerialAccess have
// the same order as the program order
type Communication [2]primitive.Access

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

type SchedPoint struct {
}

func GeneratingSchedule(knots []Knot) []SchedPoint {
	orch := orchestrator{knots: knots}
	target := orch.selectHarmoniousKnots()
	_ = target
	return nil
}

type orchestrator struct {
	// Communications that are already selected
	comms []Communication
	// Input knots
	knots []Knot
}

func (orch *orchestrator) selectHarmoniousKnots() []Knot {
	res := []Knot{}
	remaining := make([]Knot, 0, len(orch.knots))
	for _, knot := range orch.knots {
		if orch.harmoniousKnot(knot) {
			res = append(res, knot)
			orch.comms = append(orch.comms, knot[0], knot[1])
		} else {
			remaining = append(remaining, knot)
		}
	}
	orch.knots = remaining
	orch.comms = nil
	return res
}

func (orch orchestrator) harmoniousKnot(knot Knot) bool {
	for _, comm := range orch.comms {
		if knot[0].Conflict(comm) || knot[1].Conflict(comm) {
			return false
		}
	}
	return true
}

type KnotType int

const (
	KnotInvalid KnotType = iota
	KnotParallel
	KnotOverlapped
	KnotSeparated
)
