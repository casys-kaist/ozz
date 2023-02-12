package interleaving_test

import (
	"math/rand"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestToAndFromHex(t *testing.T) {
	sig := interleaving.Signal{}
	for i := 0; i < 100; i++ {
		r := rand.Uint64()
		sig[r] = struct{}{}
	}
	copied := sig.Copy()
	if diff := copied.Diff(sig); !diff.Empty() {
		t.Errorf("sig and copied are different")
	}
	data := copied.ToHex()
	recovered := interleaving.Signal{}
	recovered.FromHex(data)
	if diff1, diff2 := copied.Diff(recovered), sig.Diff(recovered); !diff1.Empty() || !diff2.Empty() {
		t.Errorf("wrong")
	}
}
