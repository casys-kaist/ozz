package interleaving

import (
	"fmt"
	"sort"
)

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
	copySortedAccs := func(accs []Access) []Access {
		cpy := make([]Access, len(accs))
		copy(cpy, accs)
		sort.Slice(cpy, func(i, j int) bool { return cpy[i].Timestamp < cpy[j].Timestamp })
		return cpy
	}
	str := fmt.Sprintf("Type: %v\nScore: %v\nCritical communication\n - %v -> %v\nPreceding insts\n", hint.Typ, hint.Score(), hint.CriticalComm.Former(), hint.CriticalComm.Latter())
	for _, acc := range copySortedAccs(hint.PrecedingInsts) {
		str += fmt.Sprintf(" - %v\n", acc)
	}
	str += "Following insts\n"
	for _, acc := range copySortedAccs(hint.FollowingInsts) {
		str += fmt.Sprintf(" - %v\n", acc)
	}
	return str
}

func (hint Hint) Score() int {
	var accs []Access
	var accTyp, score int
	switch hint.Typ {
	case TestingLoadBarrier:
		accs, accTyp = hint.FollowingInsts, TypeLoad
	case TestingStoreBarrier:
		accs, accTyp = hint.PrecedingInsts, TypeStore
	}
	for _, acc := range accs {
		if acc.Typ == uint32(accTyp) {
			score++
		}
	}
	return score
}

func (hint Hint) Coverage() Signal {
	var accs []Access
	var pivot Access
	switch hint.Typ {
	case TestingStoreBarrier:
		accs = hint.PrecedingInsts
		pivot = hint.CriticalComm.Former()
	case TestingLoadBarrier:
		accs = hint.FollowingInsts
		pivot = hint.CriticalComm.Latter()
	}
	sign := make(Signal)
	const (
		offset_bias = 0x01000193
		prime       = 0x01000193
	)
	for _, acc := range accs {
		s := uint32(offset_bias)
		s ^= pivot.Inst
		s *= prime
		s ^= acc.Inst
		s *= prime
		sign[s] = struct{}{}
	}
	return sign
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
	switch hint.Typ {
	case TestingStoreBarrier:
		return []Access{c.Former()}
	case TestingLoadBarrier:
		const callInstSize = 5
		res := []Access{c.Latter(), c.Former()}
		res[0].Inst -= callInstSize
		return res
	default:
		panic("should not reach here")
	}
}

func Select(s1, s2 []Hint) []Hint {
	// Return hints in s1 that are also contained in s2,
	s2Cov := make(Signal)
	for _, hint := range s2 {
		s2Cov.Merge(hint.Coverage())
	}
	res := []Hint{}
	for _, hint := range s1 {
		cov := hint.Coverage()
		if len(s2Cov.Intersect(cov)) != 0 {
			res = append(res, hint)
		}
	}
	return res
}
