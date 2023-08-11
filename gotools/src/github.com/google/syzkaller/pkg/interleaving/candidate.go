package interleaving

type Candidate struct {
	DelayingInst []Access
	CriticalComm Communication
}

func (cand Candidate) Invalid() bool {
	return false
}

func (cand Candidate) GenerateSchedule() []Access {
	return nil
}
