package interleaving

// TODO implement

// func initTest(t *testing.T) (*rand.Rand, int) {
// 	iters := 1000
// 	if testing.Short() {
// 		iters = 100
// 	}
// 	return rand.New(testutil.RandSource(t)), iters
// }

// func TestGenerateFlushVector(t *testing.T) {
// 	// TODO
// }

// func TestGenerateRandomFlushVector(t *testing.T) {
// 	r, iters := initTest(t)
// 	for _, test := range vecGentests {
// 		ok := false
// 		for i := 0; i < iters; i++ {
// 			vec := generateRandomFlushVector(r)
// 			t.Logf("vec :%v", vec)
// 			if len(vec) < 2 {
// 				t.Errorf("wrong, vec: %v", vec)
// 			}
// 			if checkVectorSame(t, vec, test.ans) {
// 				ok = true
// 				break
// 			}
// 		}
// 		if !ok {
// 			t.Errorf("%v: wrong, ans: %v", test.name, test.ans)
// 		}
// 	}
// }

// func Test__generatePossibleFlushVectors(t *testing.T) {
// 	check := func(vecs1, vecs2 []FlushVector) bool {
// 		for _, v1 := range vecs1 {
// 			found := false
// 			for _, v2 := range vecs2 {
// 				if len(v1) != len(v2) {
// 					continue
// 				}
// 				same := true
// 				for i := 0; i < len(v1); i++ {
// 					if v1[i] != v2[i] {
// 						same = false
// 						break
// 					}
// 				}
// 				if same {
// 					found = true
// 					break
// 				}
// 			}
// 			if !found {
// 				return false
// 			}
// 		}
// 		return true
// 	}
// 	vecs := [][]FlushVector{
// 		{},
// 		{},
// 		{{0, 1}, {1, 0}}, //2
// 		{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 1, 0}, {1, 0, 1}, {0, 1, 1}}, //3
// 		{{1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}, {0, 0, 0, 1}, {1, 1, 0, 0}, {1, 0, 1, 0}, {1, 0, 0, 1}, {0, 1, 1, 0}, {0, 1, 0, 1}, {0, 0, 1, 1}, {1, 1, 1, 0}, {1, 1, 0, 1}, {1, 0, 1, 1}, {0, 1, 1, 1}}, //4
// 	}
// 	for i := 2; i <= 4; i++ {
// 		t.Logf("combinations(%d)", i)
// 		generated := __generatePossibleFlushVectors(i)
// 		rand.Shuffle(len(generated), func(i, j int) {
// 			generated[i], generated[j] = generated[j], generated[i]
// 		})
// 		t.Logf("generated: %v", generated)
// 		t.Logf("wanted   : %v", vecs[i])
// 		if !check(generated, vecs[i]) {
// 			t.Errorf("wrong: wants: %v, generated: %v", vecs[i], generated)
// 		}
// 	}
// }

// func checkVectorSame(t *testing.T, vec, ans FlushVector) bool {
// 	if len(vec) != len(ans) {
// 		return false
// 	}
// 	ok := true
// 	for i := 0; i < len(vec) && ok; i++ {
// 		if vec[i] != ans[i] {
// 			ok = false
// 		}
// 	}
// 	if !ok {
// 		return false
// 	}
// 	return true
// }

// type testT struct {
// 	name  string
// 	hints []Segment
// 	ans   FlushVector
// }

// var vecGentests []testT = []testT{
// 	{
// 		name: "tso_test",
// 		hints: []Segment{Knot{
// 			{{Inst: 0x81a651e0, Size: 4, Typ: TypeStore, Timestamp: 1}, {Inst: 0x81a65291, Size: 4, Typ: TypeLoad, Thread: 1, Timestamp: 8}},
// 			{{Inst: 0x81a651f1, Size: 4, Typ: TypeLoad, Timestamp: 2}, {Inst: 0x81a65280, Size: 4, Typ: TypeStore, Thread: 1, Timestamp: 7}},
// 		}},
// 		ans: FlushVector{0, 1},
// 	},
// 	{
// 		name: "pso_test",
// 		hints: []Segment{Knot{
// 			{{Inst: 0x81a6167c, Size: 4, Typ: TypeStore, Timestamp: 6}, {Inst: 0x81a61ba4, Size: 4, Typ: TypeLoad, Thread: 1, Timestamp: 1750}},
// 			{{Inst: 0x81a616a6, Size: 4, Typ: TypeStore, Timestamp: 8}, {Inst: 0x81a61af7, Size: 4, Typ: TypeLoad, Thread: 1, Timestamp: 1749}},
// 		}},
// 		ans: FlushVector{0, 1, 0},
// 	},
// 	{
// 		name: "watchqueue_pipe",
// 		hints: []Segment{Knot{
// 			{{Inst: 0x81ad9a0c, Size: 8, Typ: TypeStore, Timestamp: 98}, {Inst: 0x81f83178, Size: 8, Typ: TypeLoad, Thread: 1, Timestamp: 197}},
// 			{{Inst: 0x81ad9a84, Size: 4, Typ: TypeStore, Timestamp: 102}, {Inst: 0x81f82be8, Size: 4, Typ: TypeLoad, Thread: 1, Timestamp: 191}},
// 		}},
// 		ans: FlushVector{1, 0, 0},
// 	},
// }
