package scheduler

import (
	"fmt"

	"github.com/google/syzkaller/pkg/primitive"
)

// NOTE: We infer the program order with at most two serial
// executions. Thus, we implement a heuristic of pairwise sequence
// alignment. Extend it to handle multiple sequences if we want to
// handle more complex cases (i.e., with more than two serials).

func pairwiseSequenceAlign(s1, s2 *primitive.SerialAccess) {
	// Heuristic to align two serial accesses based on instruction
	// addresses. Time and space complexity is O(n).

	aligner := aligner{s1: s1, s2: s2, windowSize: windowSize}
	aligner.pairwiseSequenceAlign()
}

type aligner struct {
	windowSize int
	po         uint32
	i1         int
	footprint1 map[uint32]int
	i2         int
	footprint2 map[uint32]int
	// inputs
	s1, s2 *primitive.SerialAccess
}

func (a aligner) pairwiseSequenceAlign() {
	a.collectFootprint()
	for a.i1 < len(*a.s1) || a.i2 < len(*a.s2) {
		// Let's consider instruction addresses only
		if a.i1 < len(*a.s1) && a.i2 < len(*a.s2) && (*a.s1)[a.i1].Inst == (*a.s2)[a.i2].Inst {
			a.adjustAccessesPO(1, 1, true)
		} else {
			// Two sequences diverges at i1 and i2. For each serials,
			// find out the number of Accesses until two sequences
			// converges again.
			diverged1, diverged2 := a.countDivergedAccessess()
			// Forward progress should be guaranteed
			if diverged1 == 0 && diverged2 == 0 || diverged1 < 0 || diverged2 < 0 {
				panic(fmt.Sprintf("wrong %d %d", diverged1, diverged2))
			}
			// (*s1)[i1:next_i1] and (*s2)[i2:next_i2] are diverged sequences.
			a.adjustAccessesPO(diverged1, diverged2, false)
		}
	}
}

func (a *aligner) collectFootprint() {
	a.footprint1 = collectFootprint(a.s1, a.windowSize)
	a.footprint2 = collectFootprint(a.s2, a.windowSize)
}

func (a *aligner) adjustAccessesPO(cnt1, cnt2 int, common bool) {
	loop := func(loopCnt int, s1, s2 bool) {
		for offset := 0; offset < loopCnt; offset++ {
			var acc1, acc2 *primitive.Access
			if s1 {
				acc1 = &(*a.s1)[a.i1+offset]
			}
			if s2 {
				acc2 = &(*a.s2)[a.i2+offset]
			}
			a.adjustPO(acc1, acc2, common)
		}
	}
	if common {
		if cnt1 != cnt2 {
			panic("wrong")
		}
		loop(cnt1, true, true)
	} else {
		loop(cnt1, true, false)
		loop(cnt2, false, true)
	}
	a.i1, a.i2 = a.i1+cnt1, a.i2+cnt2
}

func (a aligner) countDivergedAccessess() (int, int) {
	if a.i1 == len(*a.s1) {
		return 0, len(*a.s2) - a.i2
	}
	if a.i2 == len(*a.s2) {
		return len(*a.s1) - a.i1, 0
	}
	// TODO: This function calls are confusing
	d11, d21, total1 := countDivergedAccessesPivot(a.s1, a.s2, a.footprint2, a.i1, a.i2, a.windowSize)
	d22, d12, total2 := countDivergedAccessesPivot(a.s2, a.s1, a.footprint1, a.i2, a.i1, a.windowSize)
	if total1 < total2 {
		return d11, d21
	} else {
		return d12, d22
	}
}

func countDivergedAccessesPivot(pivot, counterpart *primitive.SerialAccess, footprint map[uint32]int, from1, from2, windowSize int) (diverged1, diverged2, total int) {
	defer func() {
		total = diverged1
		if total < diverged2 {
			total = diverged2
		}
	}()
	for idx1 := from1; idx1 < len(*pivot); idx1++ {
		hsh := hashWindow(pivot, idx1, windowSize)
		if idx2, ok := footprint[hsh]; ok && idx2 > from2 {
			diverged1 = idx1 - from1
			diverged2 = idx2 - from2
			return
		}
	}
	diverged1 = len(*pivot) - from1
	diverged2 = len(*counterpart) - from2
	return
}

func (a *aligner) adjustPO(acc1, acc2 *primitive.Access, common bool) {
	var ctx1, ctx2 uint32
	if common {
		ctx1, ctx2 = primitive.CommonPath, primitive.CommonPath
	} else {
		ctx1, ctx2 = 0, 1
	}
	if acc1 != nil {
		acc1.Timestamp = a.po
		acc1.Context = ctx1
	}
	if acc2 != nil {
		acc2.Timestamp = a.po
		acc2.Context = ctx2
	}
	a.po++
}

func collectFootprint(s *primitive.SerialAccess, windowSize int) map[uint32]int {
	fp := make(map[uint32]int)
	for i := 0; i < len(*s); i++ {
		hsh := hashWindow(s, i, windowSize)
		fp[hsh] = i
	}
	return fp
}

func hashWindow(s *primitive.SerialAccess, i, windowSize int) uint32 {
	end := i + windowSize
	if end > len(*s) {
		end = len(*s)
	}
	window := (*s)[i:end]
	hsh := uint32(0)
	const prime = 131071
	for _, acc := range window {
		hsh = (hsh << 1) ^ (acc.Inst * prime)
	}
	return hsh
}

const windowSize = 5
