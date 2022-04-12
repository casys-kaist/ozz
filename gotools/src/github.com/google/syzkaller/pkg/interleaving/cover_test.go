package interleaving_test

import (
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestCoverSerializeSingle(t *testing.T) {
	cover := interleaving.Cover{
		interleaving.Knot{
			{
				interleaving.Access{1, 2, 3, 4, 5, 6, 7},
				interleaving.Access{11, 12, 13, 14, 15, 16, 17},
			},
			{
				interleaving.Access{21, 22, 23, 24, 25, 26, 27},
				interleaving.Access{31, 32, 33, 34, 35, 36, 37},
			},
		},
	}
	serialized := cover.Serialize()
	deserialized := interleaving.Deserialize(serialized)
	if !coverSame(cover, deserialized) {
		t.Errorf("wrong\nOriginal:\n%v\nDeserialized:\n%v", cover, deserialized)
	}
}

func coverSame(c1, c2 interleaving.Cover) bool {
	return reflect.DeepEqual(c1, c2)
}
