package interleaving

type Candidate struct {
	DelayingInst []Access
	CriticalComm Communication
}

func (cand Candidate) Invalid() bool {
	return len(cand.DelayingInst) == 0 || cand.invalidCriticalComm()
}

func (cand Candidate) invalidCriticalComm() bool {
	c := cand.CriticalComm
	return c.Former().Inst == 0 || c.Latter().Inst == 0
}

func (cand Candidate) GenerateSchedule() []Access {
	c := cand.CriticalComm
	// NOTE: As long as we consider one critical communication for one
	// candidate, a schedule always contains one access which is the
	// first access of the critcal comm.
	return []Access{c.Former()}
}
