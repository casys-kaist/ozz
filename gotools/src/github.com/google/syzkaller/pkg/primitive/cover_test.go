package primitive_test

import (
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func TestCoverSerializeSingle(t *testing.T) {
	cover := primitive.Cover{
		primitive.Knot{
			{
				primitive.Access{1, 2, 3, 4, 5, 6, 7},
				primitive.Access{11, 12, 13, 14, 15, 16, 17},
			},
			{
				primitive.Access{21, 22, 23, 24, 25, 26, 27},
				primitive.Access{31, 32, 33, 34, 35, 36, 37},
			},
		},
	}
	serialized := cover.Serialize()
	deserialized := primitive.Deserialize(serialized)
	if !coverSame(cover, deserialized) {
		t.Errorf("wrong\nOriginal:\n%v\nDeserialized:\n%v", cover, deserialized)
	}
}

func coverSame(c1, c2 primitive.Cover) bool {
	return reflect.DeepEqual(c1, c2)
}
