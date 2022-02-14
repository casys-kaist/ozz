package scheduler

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func loadTestdata(raw []byte) (threads [2]primitive.SerialAccess, e error) {
	timestamp, thread := uint32(0), -1
	for {
		idx := bytes.IndexByte(raw, byte('\n'))
		if idx == -1 {
			break
		}
		line := raw[:idx]
		raw = raw[idx+1:]

		toks := bytes.Fields(line)
		if len(toks) < 3 {
			if bytes.HasPrefix(line, []byte("Thread")) {
				thread++
			}
			continue
		}

		var typ uint32
		if bytes.Equal(toks[2], []byte("R")) {
			typ = primitive.TypeLoad
		} else {
			typ = primitive.TypeStore
		}

		inst, err := strconv.ParseUint(string(toks[0]), 16, 64)
		if err != nil {
			e = err
			return
		}
		addr, err := strconv.ParseUint(string(toks[1][2:]), 16, 64)
		if err != nil {
			e = err
			return
		}

		acc := primitive.Access{
			Inst:      uint32(inst),
			Addr:      uint32(addr),
			Typ:       typ,
			Size:      4,
			Timestamp: timestamp,
			Thread:    uint64(thread),
		}
		threads[thread].Add(acc)
		timestamp++
	}
	return
}

func loadKnots(t *testing.T, paths []string) []primitive.Knot {
	knotter := Knotter{}
	for _, _path := range paths {
		path := filepath.Join("testdata", _path)
		data, err := ioutil.ReadFile(path)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		thrs, err := loadTestdata(data)
		if err != nil {
			t.Errorf("%v", err)
		}
		knotter.AddSequentialTrace(thrs[:])
	}
	knots := knotter.ExcavateKnots()
	t.Logf("# of knots: %d", len(knots))
	return knots
}

func checkAnswer(t *testing.T, knots []primitive.Knot, required primitive.Knot) bool {
	found := false
	for i, knot := range knots {
		t.Logf("Knot %d, type: %v", i, knot.Type())
		t.Logf("  %x (%v) --> %x (%v)", knot[0][0].Inst, knot[0][0].Timestamp, knot[0][1].Inst, knot[0][1].Timestamp)
		t.Logf("  %x (%v) --> %x (%v)", knot[1][0].Inst, knot[1][0].Timestamp, knot[1][1].Inst, knot[1][1].Timestamp)
		if knot.Same(required) {
			t.Logf("found")
			found = true
		}
	}
	return found
}
