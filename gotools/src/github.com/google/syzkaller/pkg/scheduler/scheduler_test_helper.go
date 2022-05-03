package scheduler

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/log"
)

func _loadTestdata(raw []byte) (threads [2]interleaving.SerialAccess, e error) {
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
			typ = interleaving.TypeLoad
		} else {
			typ = interleaving.TypeStore
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

		acc := interleaving.Access{
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

func loadTestdata(tb testing.TB, paths []string, knotter *Knotter) [][2]interleaving.SerialAccess {
	res := [][2]interleaving.SerialAccess{}
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

func loadKnots(t *testing.T, paths []string) []interleaving.Knot {
	knotter := Knotter{}
	loadTestdata(t, paths, &knotter)
	knotter.ExcavateKnots()
	knots0 := knotter.GetKnots()
	knots := []interleaving.Knot{}
	for _, knot0 := range knots0 {
		knots = append(knots, knot0.(interleaving.Knot))
	}
	t.Logf("# of knots: %d", len(knots))
	return knots
}

func checkAnswer(t *testing.T, knots []interleaving.Knot, required interleaving.Knot) bool {
	for _, knot := range knots {
		if ok, err := knot.Imply(required); err != nil {
			printKnot(knot)
			printKnot(required)
			panic(err)
		} else if ok {
			return true
		}
	}
	return false
}

func printKnot(knot interleaving.Knot) {
	for _, comm := range knot {
		log.Logf(0, "%v --> %v", comm.Former(), comm.Latter())
	}
}
