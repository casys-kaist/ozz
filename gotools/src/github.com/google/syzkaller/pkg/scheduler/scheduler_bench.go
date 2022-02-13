package scheduler

import (
	"testing"
)

func benchmarkExcavateKnots(b *testing.B) {
	// // data1 is larger than data2
	// path := filepath.Join("testdata", "data1")
	// data, err := ioutil.ReadFile(path)
	// if err != nil {
	// 	b.Errorf("unexpected error: %v", err)
	// }
	// thrs, err := loadTestdata(data)

	// b.Run("total", func(b *testing.B) {
	// 	b.ReportAllocs()
	// 	for i := 0; i < b.N; i++ {
	// 		ExcavateKnots(thrs[:])
	// 	}
	// })
	// knotter := knotter{
	// 	accesses:    thrs[:],
	// 	loopAllowed: loopAllowed,
	// }
	// b.Run("fastenKnots", func(b *testing.B) {
	// 	b.ReportAllocs()
	// 	for i := 0; i < b.N; i++ {
	// 		knotter.fastenKnots()
	// 	}
	// })
	// b.Run("buildAccessMap", func(b *testing.B) {
	// 	b.ReportAllocs()
	// 	for i := 0; i < b.N; i++ {
	// 		knotter.buildAccessMap()
	// 	}
	// })
	// b.Run("formCommunications", func(b *testing.B) {
	// 	b.ReportAllocs()
	// 	for i := 0; i < b.N; i++ {
	// 		knotter.formCommunications()
	// 	}
	// })
	// b.Run("formKnots", func(b *testing.B) {
	// 	b.ReportAllocs()
	// 	for i := 0; i < b.N; i++ {
	// 		knotter.formKnots()
	// 	}
	// })
}
