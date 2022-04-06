package main

import (
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/primitive"
	"github.com/google/syzkaller/prog"
)

func debugHint(tp *prog.Candidate, remaining []primitive.Segment) {
	if !_debug {
		return
	}
	before := testingHints(tp.Hint)
	after := testingHints(remaining)
	if before && !after {
		log.Logf(0, "This input should crash")
	}
}

// XXX: For the dedug purpose
func testingHints(hint []primitive.Segment) bool {
	var answer = primitive.Knot{
		{{Inst: 0x81f9ebf6, Size: 4, Typ: primitive.TypeStore}, {Inst: 0x81f9f2e8, Size: 4, Typ: primitive.TypeLoad}},
		{{Inst: 0x8d576644, Size: 4, Typ: primitive.TypeStore}, {Inst: 0x8d5924a3, Size: 4, Typ: primitive.TypeLoad}}}
	for _, knot0 := range hint {
		knot := knot0.(primitive.Knot)
		if knot.Same(answer) {
			return true
		}
	}
	return false
}

var _debug bool = true
