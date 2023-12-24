package prog

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/ssb"
)

func (p *Prog) MutateFlushVectorFromHint(r *rand.Rand, hint interleaving.Hint, randomReordering bool) {
	vec := ssb.GenerateFlushVector(r, hint, randomReordering)
	p.AttachFlushVector(vec)
}

func (p *Prog) AttachFlushVector(vec ssb.FlushVector) {
	p.FlushVector = vec
}
