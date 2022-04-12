package interleaving_test

import (
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestCommHash(t *testing.T) {
	comm0 := interleaving.Communication{{Thread: 1}, {Thread: 2}}
	hsh0 := comm0.Hash()
	comm1 := interleaving.Communication{{Thread: 2}, {Thread: 1}}
	hsh1 := comm1.Hash()
	t.Logf("hsh0: %x", hsh0)
	t.Logf("hsh1: %x", hsh1)
	if hsh0 == hsh1 {
		t.Errorf("wrong, two hash values should be different")
	}
}

func TestKnotHash(t *testing.T) {
	knot0 := interleaving.Knot{
		{{Timestamp: 0, Thread: 0}, {Timestamp: 3, Thread: 1}},
		{{Timestamp: 1, Thread: 1}, {Timestamp: 2, Thread: 0}},
	}
	hsh0 := knot0.Hash()
	knot1 := interleaving.Knot{
		{{Timestamp: 100, Thread: 0}, {Timestamp: 103, Thread: 1}},
		{{Timestamp: 101, Thread: 1}, {Timestamp: 102, Thread: 0}},
	}
	hsh1 := knot1.Hash()

	t.Logf("hsh0: %x", hsh0)
	t.Logf("hsh1: %x", hsh1)
	if hsh0 != hsh1 {
		t.Errorf("wrong, two hash values should be same")
	}
}
