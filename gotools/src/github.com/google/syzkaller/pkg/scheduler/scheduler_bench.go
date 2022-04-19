package scheduler

import (
	"reflect"
	"testing"

	"github.com/google/syzkaller/pkg/interleaving"
)

func benchmarkExcavateKnots(b *testing.B) {
	testdata := []string{
		"data1",
		"heavy_data",
	}
	for _, testdata := range testdata {
		benchmarkExcavateKnotsWithData(b, testdata)
	}
}

var testdata string

func benchmarkExcavateKnotsWithData(b *testing.B, _testdata string) {
	testdata = _testdata
	subBenchmarks := []struct {
		name string
		f    func(*testing.B)
	}{
		{"total", benchmarkTotal},
		{"fastenKnots", benchmarkFastenKnots},
		{"collectCommChans", benchmarkCollectCommChans},
		{"buildAccessMap", benchmarkBuildAccessMap},
		{"formCommunications", benchmarkFormCommunications},
		{"formKnots", benchmarkFormKnots},
	}
	for _, sub := range subBenchmarks {
		b.Run(testdata+"/"+sub.name, sub.f)
	}
}

func benchmarkTotal(b *testing.B) {
	thrs := loadTestdata(b, []string{testdata}, nil)
	thrs0 := make([][2]interleaving.SerialAccess, len(thrs))
	copy(thrs0, thrs)
	b.ReportAllocs()
	b.ResetTimer()
	total := 0
	for i := 0; i < b.N; i++ {
		knotter := &Knotter{}
		for _, seq := range thrs {
			knotter.AddSequentialTrace(seq[:])
		}
		knotter.ExcavateKnots()
		total += len(knotter.GetKnots())
	}
	b.ReportMetric(float64(total/b.N), "knots/op")
	b.StopTimer()
	if !reflect.DeepEqual(thrs0, thrs) {
		b.Fatalf("input data is corrupted")
	}
}

// Belows are sub-benchmarks with the purpose of breakdown
// benchamrkExcavateKnots. TODO: further refactoring to reduce the
// duplicated codes.
func benchmarkFastenKnots(b *testing.B) {
	knotter := &Knotter{loopAllowed: loopAllowed}
	doSubBenchmarks(b, knotter, []func(){},
		knotter.fastenKnots)
}

func benchmarkCollectCommChans(b *testing.B) {
	knotter := &Knotter{loopAllowed: loopAllowed}
	doSubBenchmarks(b, knotter, []func(){},
		knotter.collectCommChans)
}

func benchmarkBuildAccessMap(b *testing.B) {
	knotter := &Knotter{loopAllowed: loopAllowed}
	doSubBenchmarks(b, knotter, []func(){
		knotter.collectCommChans,
	}, knotter.buildAccessMap)
}

func benchmarkFormCommunications(b *testing.B) {
	knotter := &Knotter{loopAllowed: loopAllowed}
	doSubBenchmarks(b, knotter, []func(){
		knotter.collectCommChans,
		knotter.buildAccessMap,
	}, knotter.formCommunications)
}

func benchmarkFormKnots(b *testing.B) {
	knotter := &Knotter{loopAllowed: loopAllowed}
	doSubBenchmarks(b, knotter, []func(){
		knotter.collectCommChans,
		knotter.buildAccessMap,
		knotter.formCommunications,
	}, knotter.formKnots)
}

func doSubBenchmarks(b *testing.B, knotter *Knotter, prerequisites []func(), do func()) {
	thrs := loadTestdata(b, []string{testdata}, nil)
	knotter.AddSequentialTrace(thrs[0][:])
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reset(b, knotter, thrs, prerequisites)
		do()
	}
}

func reset(b *testing.B, knotter *Knotter, seqs [][2]interleaving.SerialAccess, prerequisites []func()) {
	b.StopTimer()
	knotter.commChan = nil
	knotter.accessMap = nil
	knotter.numThr = 0
	knotter.seqs = nil
	knotter.comms = nil
	knotter.knots = nil
	for _, prerequisite := range prerequisites {
		prerequisite()
	}
	b.StartTimer()
}
