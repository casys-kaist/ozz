package scheduler

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func TestPairwiseSequenceAlign(t *testing.T) {
	tests := []struct {
		serials    []interleaving.SerialAccess
		windowSize int
		ans        []interleaving.SerialAccess
	}{
		{
			[]interleaving.SerialAccess{
				{{Inst: 0}, {Inst: 3}, {Inst: 7}, {Inst: 10}},
				{{Inst: 2}, {Inst: 3}, {Inst: 10}},
			},
			1,
			[]interleaving.SerialAccess{
				{{Inst: 0}, {Inst: 3, Timestamp: 2, Context: interleaving.CommonPath}, {Inst: 7, Timestamp: 3}, {Inst: 10, Timestamp: 4, Context: interleaving.CommonPath}},
				{{Inst: 2, Timestamp: 1, Context: 1}, {Inst: 3, Timestamp: 2, Context: interleaving.CommonPath}, {Inst: 10, Timestamp: 4, Context: interleaving.CommonPath}},
			},
		},
		{
			[]interleaving.SerialAccess{
				{{Inst: 1}, {Inst: 3}, {Inst: 7}, {Inst: 10}},
				{{Inst: 2}, {Inst: 3}, {Inst: 10}},
			},
			5,
			[]interleaving.SerialAccess{
				{{Inst: 1}, {Inst: 3, Timestamp: 1}, {Inst: 7, Timestamp: 2}, {Inst: 10, Timestamp: 5, Context: interleaving.CommonPath}},
				{{Inst: 2, Timestamp: 3, Context: 1}, {Inst: 3, Timestamp: 4, Context: 1}, {Inst: 10, Timestamp: 5, Context: interleaving.CommonPath}},
			},
		},
		{
			[]interleaving.SerialAccess{
				{{Inst: 0}, {Inst: 1}},
				{{Inst: 2}, {Inst: 3}},
			},
			1,
			[]interleaving.SerialAccess{
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

func TestAlignFromRealdata(t *testing.T) {
	// data from a faild execution
	seq := loadTestdata(t, []string{"align_data"}, nil)[0]
	pairwiseSequenceAlign(&seq[0], &seq[1])

	s0, s1 := seq[0], seq[1]
	for i0, i1 := 0, 0; i0 < len(s0) || i1 < len(s1); {
		if s0[i0].Context != interleaving.CommonPath || s1[i1].Context != interleaving.CommonPath {
			if s0[i0].Context != interleaving.CommonPath {
				i0++
			}
			if s1[i1].Context != interleaving.CommonPath {
				i1++
			}
			continue
		}
		if s0[i0].Inst != s1[i1].Inst {
			t.Errorf("Two accesses' instruction addresses are different\n%v\n%v", s0[i0], s1[i1])
		}
		i0, i1 = i0+1, i1+1
	}
	check := func(s interleaving.SerialAccess, id int) {
		for i, acc := range s {
			if i > 0 && acc.Timestamp <= s[i-1].Timestamp {
				t.Errorf("%d: PO is not monotonically increasing\n%v\n%v", id, acc, s[i-1])
			}
			if acc.Context != uint32(id) && acc.Context != interleaving.CommonPath {
				t.Errorf("%d: wrong context\n%v", id, acc)
			}
		}
	}
	check(s0, 0)
	check(s1, 1)
}

func _toString(serials []interleaving.SerialAccess) (str string) {
	for i, serial := range serials {
		str += fmt.Sprintf("Serial #%d\n", i)
		for _, acc := range serial {
			str += fmt.Sprintf("%v\n", acc)
		}
	}
	return
}
