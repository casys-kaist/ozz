package interleaving

type Hint struct {
	DelayingInst []Access
	SomeInst     []Access
	CriticalComm Communication
}

func (hint Hint) Invalid() bool {
	return len(hint.DelayingInst) == 0 || hint.invalidCriticalComm()
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
