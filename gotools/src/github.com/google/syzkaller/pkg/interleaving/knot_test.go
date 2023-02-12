package interleaving_test

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestKnotType(t *testing.T) {
	tests := []struct {
		knot interleaving.Knot
		ans  interleaving.KnotType
	}{
		{
			[2]interleaving.Communication{
				{{Timestamp: 0, Thread: 0}, {Timestamp: 2, Thread: 1}},
				{{Timestamp: 1, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			interleaving.KnotParallel,
		},
		{
			[2]interleaving.Communication{
				{{Timestamp: 2, Thread: 1}, {Timestamp: 0, Thread: 0}},
				{{Timestamp: 3, Thread: 1}, {Timestamp: 1, Thread: 0}},
			},
			interleaving.KnotParallel,
		},
		{
			[2]interleaving.Communication{
				{{Timestamp: 0, Thread: 0}, {Timestamp: 3, Thread: 1}},
				{{Timestamp: 1, Thread: 1}, {Timestamp: 2, Thread: 0}},
			},
			interleaving.KnotOverlapped,
		},
		{
			[2]interleaving.Communication{
				{{Timestamp: 3, Thread: 1}, {Timestamp: 0, Thread: 0}},
				{{Timestamp: 2, Thread: 0}, {Timestamp: 1, Thread: 1}},
			},
			interleaving.KnotInvalid,
		},
		{
			[2]interleaving.Communication{
				{{Timestamp: 0, Thread: 1}, {Timestamp: 2, Thread: 0}},
				{{Timestamp: 1, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			interleaving.KnotOverlapped,
		},
		{
			[2]interleaving.Communication{
				{{Timestamp: 0, Thread: 1}, {Timestamp: 1, Thread: 0}},
				{{Timestamp: 2, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			interleaving.KnotSeparated,
		},
		{
			[2]interleaving.Communication{
				{{Timestamp: 2, Thread: 0}, {Timestamp: 3, Thread: 1}},
				{{Timestamp: 0, Thread: 1}, {Timestamp: 1, Thread: 0}},
			},
			interleaving.KnotSeparated,
		},
	}
	for _, test := range tests {
		knot := test.knot
		if got := knot.Type(); test.ans != got {
			t.Errorf("wrong, expected %v, got %v", test.ans, got)
		}
	}
}

func TestKnotImply(t *testing.T) {
	var knot = interleaving.Knot{
		{{Inst: 0x8bbb79d6, Size: 4, Typ: interleaving.TypeLoad}, {Inst: 0x8bbca80b, Size: 4, Typ: interleaving.TypeStore, Thread: 1}},
		{{Inst: 0x8bbc9093, Size: 4, Typ: interleaving.TypeLoad, Thread: 1}, {Inst: 0x8bbb75a0, Size: 4, Typ: interleaving.TypeStore}}}
	if ok, _ := knot.Imply(knot); !ok {
		t.Errorf("Imply should return true for the same knot")
	}
}
