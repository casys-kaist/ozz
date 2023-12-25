package scheduler

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestLockContending(t *testing.T) {
	tests := []struct {
		seq []interleaving.SerialAccess
		num int
	}{
		{
			seq: []interleaving.SerialAccess{
				{{0, 1, 0, interleaving.TypeLockAcquire, 0, 0}, {0x1, 0xabcd, 8, interleaving.TypeStore, 1, 0}, {0, 1, 0, interleaving.TypeLockRelease, 2, 0}},
				{{0, 1, 0, interleaving.TypeLockAcquire, 3, 1}, {0x1, 0xabcd, 8, interleaving.TypeLoad, 4, 1}, {0, 1, 0, interleaving.TypeLockRelease, 5, 1}},
			},
			num: 0,
		},
		{
			seq: []interleaving.SerialAccess{
				{{0, 1, 0, interleaving.TypeLockAcquire, 0, 0}, {0x1, 0xabcd, 8, interleaving.TypeStore, 1, 0}, {0, 1, 0, interleaving.TypeLockRelease, 2, 0}},
				{{0, 2, 0, interleaving.TypeLockAcquire, 3, 1}, {0x1, 0xabcd, 8, interleaving.TypeLoad, 4, 1}, {0, 2, 0, interleaving.TypeLockRelease, 5, 1}},
			},
			num: 1,
		},
	}
	for _, test := range tests {
		printSeq(t, test.seq)
		knotter := Knotter{}
		knotter.loopAllowed = loopAllowed
		knotter.AddSequentialTrace(test.seq)
		knotter.collectCommChans()
		knotter.buildAccessMap()
		knotter.annotateLocks()
		knotter.formCommunications()
		numComms := len(knotter.comms)
		for tid, locks := range knotter.locks {
			t.Logf("tid: %d, locks: %v", tid, locks)
		}
		if test.num != numComms {
			t.Errorf("Wrong, ans: %d, got: %d", test.num, numComms)
		}
	}
}

func printSeq(t *testing.T, seq []interleaving.SerialAccess) {
	for i, serial := range seq {
		t.Logf("%d-serial", i)
		for _, acc := range serial {
			t.Logf("%v", acc)
		}
	}
}
