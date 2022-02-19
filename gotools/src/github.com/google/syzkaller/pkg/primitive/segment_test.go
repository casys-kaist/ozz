package primitive_test

import (
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func TestHash(t *testing.T) {
	comm0 := primitive.Communication{{Timestamp: 1}, {Timestamp: 2}}
	hsh0 := comm0.Hash()
	comm1 := primitive.Communication{{Timestamp: 2}, {Timestamp: 1}}
	hsh1 := comm1.Hash()
	t.Logf("hsh0: %x", hsh0)
	t.Logf("hsh1: %x", hsh1)
	if hsh0 == hsh1 {
		t.Errorf("wrong, two hash values should be different")
	}
}
