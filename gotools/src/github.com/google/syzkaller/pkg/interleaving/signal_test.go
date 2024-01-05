package interleaving_test

import (
	"math/rand"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestToAndFromHex(t *testing.T) {
	sig := interleaving.Signal{}
	for i := 0; i < 100; i++ {
		r := rand.Uint32()
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

func TestIntersect(t *testing.T) {
	s1 := interleaving.Signal{}
	for i := uint32(0); i < 100; i++ {
		s1[i] = struct{}{}
	}
	s2 := interleaving.Signal{}
	for i := uint32(0); i < 100; i += 2 {
		s2[i] = struct{}{}
	}
	sign := s1.Intersect(s2)
	if sign.Len() != 50 {
		t.Errorf("Wrong length, expected: 50, got %d", sign.Len())
	}
	for i := uint32(0); i < 100; i += 2 {
		if _, ok := sign[i]; !ok {
			t.Errorf("Wrong, %d is missing", i)
		}
	}
}

func TestIntersectRandom(t *testing.T) {
	genRand := func() interleaving.Signal {
		sig := interleaving.Signal{}
		for i := 0; i < 100; i++ {
			r := rand.Uint32()
			sig[r] = struct{}{}
		}
		return sig
	}
	s1 := genRand()
	s2 := genRand()
	s1s2 := s1.Intersect(s2)
	s2s1 := s2.Intersect(s1)
	if s1s2.Len() != s2s1.Len() {
		t.Errorf("Wrong, s1s2: %d, s2s1: %d", s1s2.Len(), s2s1.Len())
	}
	if s1s2.Diff(s2s1).Len() != 0 || s2s1.Diff(s1s2).Len() != 0 {
		t.Errorf("Wrong")
	}
}
