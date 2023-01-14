package prog

import "github.com/google/syzkaller/pkg/ssb"

func (p *Prog) AttachFlushVector(vec ssb.FlushVector) {
	p.FlushVector = make(ssb.FlushVector, len(vec))
	copy(p.FlushVector, vec)
}
