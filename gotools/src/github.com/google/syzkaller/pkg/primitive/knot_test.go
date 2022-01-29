package primitive_test

import (
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func TestKnotType(t *testing.T) {
	tests := []struct {
		knot primitive.Knot
		ans  primitive.KnotType
	}{
		{
			[2]primitive.Communication{
				{{Timestamp: 0, Thread: 0}, {Timestamp: 2, Thread: 1}},
				{{Timestamp: 1, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			primitive.KnotParallel,
		},
		{
			[2]primitive.Communication{
				{{Timestamp: 2, Thread: 1}, {Timestamp: 0, Thread: 0}},
				{{Timestamp: 3, Thread: 1}, {Timestamp: 1, Thread: 0}},
			},
			primitive.KnotParallel,
		},
		{
			[2]primitive.Communication{
				{{Timestamp: 0, Thread: 0}, {Timestamp: 3, Thread: 1}},
				{{Timestamp: 1, Thread: 1}, {Timestamp: 2, Thread: 0}},
			},
			primitive.KnotOverlapped,
		},
		{
			[2]primitive.Communication{
				{{Timestamp: 3, Thread: 1}, {Timestamp: 0, Thread: 0}},
				{{Timestamp: 2, Thread: 0}, {Timestamp: 1, Thread: 1}},
			},
			primitive.KnotInvalid,
		},
		{
			[2]primitive.Communication{
				{{Timestamp: 0, Thread: 1}, {Timestamp: 2, Thread: 0}},
				{{Timestamp: 1, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			primitive.KnotOverlapped,
		},
		{
			[2]primitive.Communication{
				{{Timestamp: 0, Thread: 1}, {Timestamp: 1, Thread: 0}},
				{{Timestamp: 2, Thread: 0}, {Timestamp: 3, Thread: 1}},
			},
			primitive.KnotSeparated,
		},
		{
			[2]primitive.Communication{
				{{Timestamp: 2, Thread: 0}, {Timestamp: 3, Thread: 1}},
				{{Timestamp: 0, Thread: 1}, {Timestamp: 1, Thread: 0}},
			},
			primitive.KnotSeparated,
		},
	}
	for _, test := range tests {
		knot := test.knot
		if got := knot.Type(); test.ans != got {
			t.Errorf("wrong, expected %v, got %v", test.ans, got)
		}
	}
}
