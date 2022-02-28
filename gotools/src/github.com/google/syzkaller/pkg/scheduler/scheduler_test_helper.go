package scheduler

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/syzkaller/pkg/primitive"
)

func _loadTestdata(raw []byte) (threads [2]primitive.SerialAccess, e error) {
	timestamp, thread, serialID := uint32(0), -1, -1
	for {
		idx := bytes.IndexByte(raw, byte('\n'))
		if idx == -1 {
			break
		}
		line := raw[:idx]
		raw = raw[idx+1:]

		toks := bytes.Fields(line)
		if len(toks) < 3 {
			serialID++
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

		size := uint64(4)
		if len(toks) > 3 {
			size0, err := strconv.ParseUint(string(toks[3]), 10, 64)
			if err != nil {
				e = err
				return
			}
			size = size0
		}

		acc := primitive.Access{
			Inst:      uint32(inst),
			Addr:      uint32(addr),
			Typ:       typ,
			Size:      uint32(size),
			Timestamp: timestamp,
			Thread:    uint64(thread),
		}
		threads[serialID].Add(acc)
		timestamp++
	}
	return
}

func loadTestdata(tb testing.TB, paths []string, knotter *Knotter) [][2]primitive.SerialAccess {
	res := [][2]primitive.SerialAccess{}
	for _, _path := range paths {
		path := filepath.Join("testdata", _path)
		data, err := ioutil.ReadFile(path)
		if err != nil {
			tb.Errorf("unexpected error: %v", err)
		}
		thrs, err := _loadTestdata(data)
		if err != nil {
			tb.Errorf("%v", err)
		}
		res = append(res, thrs)
		if knotter != nil {
			knotter.AddSequentialTrace(thrs[:])
		}
	}
	return res
}

func loadKnots(t *testing.T, paths []string) []primitive.Knot {
	knotter := Knotter{}
	loadTestdata(t, paths, &knotter)
	knotter.ExcavateKnots()
	knots0 := knotter.GetKnots()
	knots := []primitive.Knot{}
	for _, knot0 := range knots0 {
		knots = append(knots, knot0.(primitive.Knot))
	}
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
