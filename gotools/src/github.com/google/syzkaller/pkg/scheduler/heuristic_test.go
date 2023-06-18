package scheduler

import "testing"

func TestChunknize(t *testing.T) {
	seq := loadTestdata(t, []string{"pso_test"}, nil)[0]
	for i, serial := range seq {
		t.Logf("Chunknizing %d-th serial", i)
		chunks := chunkize(serial)
		t.Logf("# of chunks: %d", len(chunks))
		for i, chunk := range chunks {
			t.Logf("%d-th chunk", i)
			for _, acc := range chunk {
				t.Logf("%v", acc)
			}
		}
	}
}
