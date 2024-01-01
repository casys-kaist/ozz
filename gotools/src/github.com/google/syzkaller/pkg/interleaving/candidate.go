package interleaving

import "fmt"

type Hint struct {
	PrecedingInsts []Access
	FollowingInsts []Access
	CriticalComm   Communication
	Typ            HintType
}

type HintType bool

func (typ HintType) String() string {
	if typ == TestingStoreBarrier {
		return "Store reordering"
	} else {
		return "Load reordering"
	}
}

const (
	TestingStoreBarrier = true
	TestingLoadBarrier  = false
)

func (hint Hint) String() string {
	// Mostly for debugging
	str := fmt.Sprintf("Type: %v\nCritical communication\n - %v -> %v\nPreceding insts\n", hint.Typ, hint.CriticalComm.Former(), hint.CriticalComm.Latter())
	for _, acc := range hint.PrecedingInsts {
		str += fmt.Sprintf(" - %v\n", acc)
	}
	str += "Following insts\n"
	for _, acc := range hint.FollowingInsts {
		str += fmt.Sprintf(" - %v\n", acc)
	}
	return str
}

func (hint Hint) Invalid() bool {
	return len(hint.PrecedingInsts) == 0 || len(hint.FollowingInsts) == 0 || hint.invalidCriticalComm()
}

func (hint Hint) invalidCriticalComm() bool {
	c := hint.CriticalComm
	return c.Former().Inst == 0 || c.Latter().Inst == 0
}

func (hint Hint) GenerateSchedule() []Access {
	c := hint.CriticalComm
	// NOTE: As long as we consider one critical communication for one
	// hintidate, a schedule always contains one access which is the
	// first access of the critcal comm.
	return []Access{c.Former()}
}
