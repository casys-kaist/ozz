package scheduler

import "github.com/google/syzkaller/pkg/interleaving"

type chunk []interleaving.Access

func chunkize(serial interleaving.SerialAccess) []chunk {
	chunks := []chunk{}
	start := 0
	size := 0
	create := false
	for i, acc := range serial {
		if acc.Typ == interleaving.TypeFlush {
			size = i - start
			create = true
		} else if i == len(serial)-1 {
			size = len(serial) - start
			create = true
		}

		if create {
			if size > 1 {
				new := append(chunk{}, serial[start:i]...)
				chunks = append(chunks, new)
			}
			start = i + 1
			create = false
		}
	}
	return chunks
}
