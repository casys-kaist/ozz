package scheduler

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func TestPairwiseSequenceAlign(t *testing.T) {
	tests := []struct {
		serials    []primitive.SerialAccess
		windowSize int
		ans        []primitive.SerialAccess
	}{
		{
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 3}, {Inst: 7}, {Inst: 10}},
				{{Inst: 2}, {Inst: 3}, {Inst: 10}},
			},
			1,
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 3, Timestamp: 2, Context: primitive.CommonPath}, {Inst: 7, Timestamp: 3}, {Inst: 10, Timestamp: 4, Context: primitive.CommonPath}},
				{{Inst: 2, Timestamp: 1, Context: 1}, {Inst: 3, Timestamp: 2, Context: primitive.CommonPath}, {Inst: 10, Timestamp: 4, Context: primitive.CommonPath}},
			},
		},
		{
			[]primitive.SerialAccess{
				{{Inst: 1}, {Inst: 3}, {Inst: 7}, {Inst: 10}},
				{{Inst: 2}, {Inst: 3}, {Inst: 10}},
			},
			5,
			[]primitive.SerialAccess{
				{{Inst: 1}, {Inst: 3, Timestamp: 1}, {Inst: 7, Timestamp: 2}, {Inst: 10, Timestamp: 5, Context: primitive.CommonPath}},
				{{Inst: 2, Timestamp: 3, Context: 1}, {Inst: 3, Timestamp: 4, Context: 1}, {Inst: 10, Timestamp: 5, Context: primitive.CommonPath}},
			},
		},
		{
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 1}},
				{{Inst: 2}, {Inst: 3}},
			},
			1,
			[]primitive.SerialAccess{
				{{Inst: 0}, {Inst: 1, Timestamp: 1}},
				{{Inst: 2, Context: 1, Timestamp: 2}, {Inst: 3, Context: 1, Timestamp: 3}},
			},
		},
	}

	for i, test := range tests {
		aligner := aligner{s1: &test.serials[0], s2: &test.serials[1], windowSize: test.windowSize}
		aligner.pairwiseSequenceAlign()
		if !reflect.DeepEqual(test.serials, test.ans) {
			t.Errorf("#%d: wrong\nexpected: %v\ngot: %v", i, _toString(test.ans), _toString(test.serials))
		}
	}
}

func _toString(serials []primitive.SerialAccess) (str string) {
	for i, serial := range serials {
		str += fmt.Sprintf("Serial #%d\n", i)
		for _, acc := range serial {
			str += fmt.Sprintf("%v\n", acc)
		}
	}
	return
}
