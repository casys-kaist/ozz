package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestExtractCriticalCommunication(t *testing.T) {
	tests := []struct {
		knot interleaving.Knot
		ans  []interleaving.Communication
	}{
		{
			knot: interleaving.Knot{
				{{Inst: 0x81a6167c, Size: 4, Typ: interleaving.TypeStore, Timestamp: 6}, {Inst: 0x81a61ba4, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1750}},
				{{Inst: 0x81a616a6, Size: 4, Typ: interleaving.TypeStore, Timestamp: 8}, {Inst: 0x81a61af7, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1749}}},
			ans: []interleaving.Communication{
				{{Inst: 0x81a616a6, Size: 4, Typ: interleaving.TypeStore, Timestamp: 8}, {Inst: 0x81a61af7, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 1749}},
			},
		},
		{
			knot: interleaving.Knot{
				{{Inst: 0x81a651e0, Size: 4, Typ: interleaving.TypeStore, Timestamp: 1}, {Inst: 0x81a65291, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 8}},
				{{Inst: 0x81a651f1, Size: 4, Typ: interleaving.TypeLoad, Timestamp: 2}, {Inst: 0x81a65280, Size: 4, Typ: interleaving.TypeStore, Thread: 1, Timestamp: 7}},
			},
			ans: []interleaving.Communication{
				{{Inst: 0x81a651e0, Size: 4, Typ: interleaving.TypeStore, Timestamp: 1}, {Inst: 0x81a65291, Size: 4, Typ: interleaving.TypeLoad, Thread: 1, Timestamp: 8}},
				{{Inst: 0x81a651f1, Size: 4, Typ: interleaving.TypeLoad, Timestamp: 2}, {Inst: 0x81a65280, Size: 4, Typ: interleaving.TypeStore, Thread: 1, Timestamp: 7}},
			},
		},
	}

	for _, test := range tests {
		comms := extractCriticalCommunication(test.knot)
		if len(comms) != len(test.ans) {
			t.Errorf("wrong")
		}
		for _, comm := range comms {
			ok := false
			for _, a := range test.ans {
				if comm.Hash() == a.Hash() {
					ok = true
				}
			}
			if !ok {
				t.Errorf("wrong")
			}
		}
	}
}

func getOverallTest() (knots [][]interleaving.Segment, classified [][]interleaving.Segment, candidates []interleaving.Segment) {
	return [][]interleaving.Segment{{
			interleaving.Knot{
				{{Inst: 0x1, Timestamp: 0x1, Typ: interleaving.TypeStore}, {Inst: 0x6, Timestamp: 0x6, Typ: interleaving.TypeLoad}},
				{{Inst: 0x3, Timestamp: 0x3, Typ: interleaving.TypeStore}, {Inst: 0x4, Timestamp: 0x4, Typ: interleaving.TypeLoad}},
			},
			interleaving.Knot{
				{{Inst: 0x2, Timestamp: 0x2, Typ: interleaving.TypeStore}, {Inst: 0x5, Timestamp: 0x5, Typ: interleaving.TypeLoad}},
				{{Inst: 0x3, Timestamp: 0x3, Typ: interleaving.TypeStore}, {Inst: 0x4, Timestamp: 0x4, Typ: interleaving.TypeLoad}},
			},
			interleaving.Knot{
				{{Inst: 0x1, Timestamp: 0x1, Typ: interleaving.TypeStore}, {Inst: 0x6, Timestamp: 0x6, Typ: interleaving.TypeLoad}},
				{{Inst: 0x2, Timestamp: 0x2, Typ: interleaving.TypeStore}, {Inst: 0x5, Timestamp: 0x5, Typ: interleaving.TypeLoad}},
			},
			interleaving.Knot{
				{{Inst: 0x7, Timestamp: 0x7, Typ: interleaving.TypeStore}, {Inst: 0x9, Timestamp: 0x9, Typ: interleaving.TypeLoad}},
				{{Inst: 0x8, Timestamp: 0x8, Typ: interleaving.TypeLoad}, {Inst: 0x10, Timestamp: 0x10, Typ: interleaving.TypeStore}},
			},
		}}, [][]interleaving.Segment{
			{
				interleaving.Knot{
					{{Inst: 0x1, Timestamp: 0x1, Typ: interleaving.TypeStore}, {Inst: 0x6, Timestamp: 0x6, Typ: interleaving.TypeLoad}},
					{{Inst: 0x3, Timestamp: 0x3, Typ: interleaving.TypeStore}, {Inst: 0x4, Timestamp: 0x4, Typ: interleaving.TypeLoad}},
				},
				interleaving.Knot{
					{{Inst: 0x2, Timestamp: 0x2, Typ: interleaving.TypeStore}, {Inst: 0x5, Timestamp: 0x5, Typ: interleaving.TypeLoad}},
					{{Inst: 0x3, Timestamp: 0x3, Typ: interleaving.TypeStore}, {Inst: 0x4, Timestamp: 0x4, Typ: interleaving.TypeLoad}},
				},
			},
			{
				interleaving.Knot{
					{{Inst: 0x1, Timestamp: 0x1, Typ: interleaving.TypeStore}, {Inst: 0x6, Timestamp: 0x6, Typ: interleaving.TypeLoad}},
					{{Inst: 0x2, Timestamp: 0x2, Typ: interleaving.TypeStore}, {Inst: 0x5, Timestamp: 0x5, Typ: interleaving.TypeLoad}},
				},
			},
			{
				interleaving.Knot{
					{{Inst: 0x7, Timestamp: 0x7, Typ: interleaving.TypeStore}, {Inst: 0x9, Timestamp: 0x9, Typ: interleaving.TypeLoad}},
					{{Inst: 0x8, Timestamp: 0x8, Typ: interleaving.TypeLoad}, {Inst: 0x10, Timestamp: 0x10, Typ: interleaving.TypeStore}},
				},
			},
			{
				interleaving.Knot{
					{{Inst: 0x7, Timestamp: 0x7, Typ: interleaving.TypeStore}, {Inst: 0x9, Timestamp: 0x9, Typ: interleaving.TypeLoad}},
					{{Inst: 0x8, Timestamp: 0x8, Typ: interleaving.TypeLoad}, {Inst: 0x10, Timestamp: 0x10, Typ: interleaving.TypeStore}},
				},
			},
		}, []interleaving.Segment{
			interleaving.Candidate{},
		}
}

func TestClassifySegments(t *testing.T) {
	knots, ans, _ := getOverallTest()
	samegroup := func(g0, g1 []interleaving.Segment) bool {
		for _, s0 := range g0 {
			ok := false
			for _, s1 := range g1 {
				if s0.Hash() == s1.Hash() {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}
	used := make(map[int]struct{})
	classified := classifySegments(knots)
	for _, group := range classified {
		ok := false
		for i, group0 := range ans {
			if _, u := used[i]; !u && samegroup(group, group0) {
				ok = true
				used[i] = struct{}{}
				break
			}
		}
		if !ok {
			t.Errorf("failed")
		}
	}
}

func TestConstructCandidate(t *testing.T) {
	knots, _, ans := getOverallTest()
	cands := aggregateRawCandidates(knots)
	for _, cand := range cands {
		ok := false
		for _, a := range ans {
			if cand.Hash() == a.Hash() {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("failed")
		}
	}
}
