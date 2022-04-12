package signal

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func initReadFrom(rf ReadFrom, data [][2]uint32) {
	for _, d := range data {
		rf.Add(d[0], d[1])
	}
}

func TestAdd(t *testing.T) {
	rf := NewReadFrom()
	data := [][2]uint32{
		{1, 2},
		{2, 3},
	}
	initReadFrom(rf, data)

	for _, d := range data {
		k := makeKey(d[0], d[1])
		if !rf.containKey(k) {
			t.Errorf("missing %d, %d", d[0], d[1])
		}
	}
}

func TestLen(t *testing.T) {
	rf := NewReadFrom()
	data := [][2]uint32{
		{1, 2},
		{2, 3},
		{2, 3}, // duplicate
		{3, 4},
	}
	initReadFrom(rf, data)
	if rf.Len() != len(data)-1 {
		t.Errorf("wrong length: expected %d, got %d",
			len(data)-1, rf.Len())
	}
}

func TestMerge(t *testing.T) {
	rf1, rf2 := NewReadFrom(), NewReadFrom()
	data1 := [][2]uint32{
		{1, 2},
		{2, 3},
	}
	data2 := [][2]uint32{
		{100, 101},
		{102, 103},
	}

	initReadFrom(rf1, data1)
	initReadFrom(rf2, data2)

	rf1.Merge(rf2)

	check := func(data [][2]uint32) {
		for _, d := range data {
			k := makeKey(d[0], d[1])
			if !rf1.containKey(k) {
				t.Errorf("missing %d, %d", d[0], d[1])
			}
		}
	}
	check(data1)
	check(data2)
}

func TestDiff(t *testing.T) {
	rf1, rf2 := NewReadFrom(), NewReadFrom()
	data1 := [][2]uint32{
		{1, 2},
		{2, 3},
	}
	data2 := [][2]uint32{
		{1, 2},
		{102, 103},
	}

	initReadFrom(rf1, data1)
	initReadFrom(rf2, data2)

	diff := rf1.Diff(rf2)
	if !diff.Contain(102, 103) {
		t.Errorf("wrong: does not contain 102, 103")
	}
	if diff.Contain(1, 2) {
		t.Errorf("wrong: contains 1, 2")
	}
	if diff.Contain(2, 3) {
		t.Errorf("wrong: contains 2, 3")
	}
}

func TestCopy(t *testing.T) {
	rf := NewReadFrom()
	data := [][2]uint32{
		{1, 2},
		{2, 3},
	}
	initReadFrom(rf, data)
	copied := rf.Copy()
	for _, d := range data {
		if !copied.Contain(d[0], d[1]) {
			t.Errorf("missing %d, %v", d[0], d[1])
		}
	}
}

func TestFromEpoch(t *testing.T) {
	epoch1, epoch2 := uint64(1), uint64(2)
	if res := FromEpoch(epoch1, epoch2); res != Before {
		t.Fatalf("wrong: expected %d, got %d", Before, res)
	}
	epoch1, epoch2 = uint64(1), uint64(1)
	if res := FromEpoch(epoch1, epoch2); res != Parallel {
		t.Fatalf("wrong: expected %d, got %d", Parallel, res)
	}
	epoch1, epoch2 = uint64(2), uint64(1)
	if res := FromEpoch(epoch1, epoch2); res != After {
		t.Fatalf("wrong: expected %d, got %d", After, res)
	}
}

func TestFlatting(t *testing.T) {
	rf := NewReadFrom()
	data := [][2]uint32{
		{1, 2},
		{2, 3},
		{2, 3}, // duplicate
		{3, 4},
	}
	initReadFrom(rf, data)
	flat := rf.Flatting()
	if l := len(flat); l != 4 {
		t.Errorf("wrong flatting len, expected %d, got %d", 4, l)
	}
}

func testFromAccesses(t *testing.T) {
	dat, err := ioutil.ReadFile("testdata/accesses.dat")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	type x struct {
		name   string
		access []interleaving.Access
	}

	// build accesses from accesses.dat
	idx := -1
	acc := []x{}
	for _, line := range strings.Split(string(dat), "\n") {
		if len(line) == 0 {
			continue
		}
		prefix := "call: "
		if strings.HasPrefix(line, prefix) {
			call := line[len(prefix):]
			acc = append(acc, x{name: call})
			idx++
			continue
		}
		u := []uint32{}
		for _, tok := range strings.Fields(line) {
			if n, err := strconv.ParseUint(tok, 16, 32); err != nil {
				t.Errorf("parsing error: %v", err)
			} else {
				u = append(u, uint32(n))
			}
		}
		acc[idx].access = append(acc[idx].access, interleaving.Access{u[0], u[1], u[2], u[3], u[4], 0 /*, uint64(idx)*/})
	}

	// let's compare the built accesses and test data
	ncalls := len(acc)
	for i := 0; i < ncalls; i++ {
		for j := 0; j < ncalls; j++ {
			// check file existence
			fn := filepath.Join("testdata", acc[i].name+"_"+acc[j].name+"_rf.dat")
			_, err := os.Stat(fn)
			exist := err == nil
			if i >= j {
				if exist {
					t.Fatalf("unexpected file exists. check gen.py: %v", fn)
				}
				continue
			} else if !exist {
				t.Fatalf("file should exist: %v", fn)
			}

			// read the answer file
			b, err := ioutil.ReadFile(fn)
			if err != nil {
				t.Errorf("unexpected error while reading a file: %v, %v", fn, err)
			}
			ans := map[key]struct{}{}
			serialAns := make(map[uint32]struct{})
			for _, line := range strings.Split(string(b), "\n") {
				if len(line) == 0 {
					continue
				}
				toks := strings.Fields(line)
				a, err := strconv.ParseUint(toks[0], 16, 32)
				if err != nil {
					t.Errorf("parsing error: %v", err)
				}
				b, err := strconv.ParseUint(toks[1], 16, 32)
				if err != nil {
					t.Errorf("parsing error: %v", err)
				}
				ans[key{from: uint32(a), to: uint32(b)}] = struct{}{}
				serialAns[uint32(a)] = struct{}{}
				serialAns[uint32(b)] = struct{}{}
			}

			// build read-from from accesses
			rf, serial := FromAccesses(acc[i].access, acc[j].access, FromEpoch(uint64(i), uint64(j)))
			for k := range rf {
				if _, ok := ans[k]; !ok {
					t.Errorf("wrong %s, %x %x", fn, k.from, k.to)
				}
			}
			if len(ans) != len(rf) {
				t.Errorf("missing read-from %s, len(ans): %v, len(rf): %v",
					fn, len(ans), len(rf))
			}
			for _, acc := range serial {
				if _, ok := serialAns[acc.Inst]; !ok {
					t.Errorf("wrong serial: missing %v", acc.Inst)
				}
			}
		}
	}
}

func TestSplit(t *testing.T) {
	rf := NewReadFrom()
	data := [][2]uint32{
		{1, 2},
		{2, 3},
		{4, 5},
		{7, 6},
	}
	initReadFrom(rf, data)
	splited := rf.Split(2)
	if len(splited) != 2 {
		t.Errorf("wrong splited: length %d", len(splited))
	}
	if len(rf) != 2 {
		t.Errorf("wrong rf: length %d", len(rf))
	}
	merged := NewReadFrom()
	merged.Merge(rf)
	merged.Merge(splited)
	if len(merged) != len(data) {
		t.Errorf("wrong merged: length %d", len(merged))
	}
	for _, d := range data {
		if !merged.Contain(d[0], d[1]) {
			t.Errorf("merged does not contain %d %d", d[0], d[1])
		}
	}
}

func TestSerializeReadFrom(t *testing.T) {
	rf := NewReadFrom()
	data := [][2]uint32{
		{1, 2},
		{2, 3},
		{4, 5},
		{7, 6},
	}
	initReadFrom(rf, data)
	serial := rf.Serialize()
	deserial := serial.Deserialize()
	if deserial.Len() != rf.Len() {
		t.Errorf("wrong length: %d %d", deserial.Len(), rf.Len())
	}
	for k := range deserial {
		if !rf.containKey(k) {
			t.Errorf("missing: %v", k)
		}
	}
}
