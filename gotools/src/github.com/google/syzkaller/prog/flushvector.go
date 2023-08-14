package prog

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/ssb"
)

func (p *Prog) MutateFlushVectorFromCandidate(r *rand.Rand, cand interleaving.Candidate) {
	vec := ssb.GenerateFlushVector(r, cand)
	p.AttachFlushVector(vec)
}

func (p *Prog) AttachFlushVector(vec ssb.FlushVector) {
	p.FlushVector = vec
}
