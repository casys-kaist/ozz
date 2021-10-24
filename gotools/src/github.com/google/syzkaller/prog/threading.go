package prog

import "github.com/google/syzkaller/pkg/log"

type RacingCalls struct {
	// Calls represents a set of prog.Calls that will be executed in
	// parallel.
	// TODO: It might be useful when we run multiple sets of
	// prog.Calls in parallel altogether. Fix this after improving
	// Threading(). See TODO's in Threading().
	Calls []int
}

func (p *Prog) Threading(calls RacingCalls) {
	// TODO: Current implementation is the Razzer's threading
	// mechanism. I think we can do better. Improve
	// Fuzzer.identifyRacingCalls() and this function together.

	if len(calls.Calls) != 2 {
		// TODO: Razzer's requirement 1. Razzer runs only two syscalls
		// in parallel.
		log.Fatalf("wrong racing calls: %d", len(calls.Calls))
	}

	idx1, idx2 := calls.Calls[0], calls.Calls[1]
	epoch1, epoch2 := p.Calls[idx1].Epoch, p.Calls[idx2].Epoch
	if epoch1 > epoch2 {
		epoch1, epoch2 = epoch2, epoch1
		idx1, idx2 = idx2, idx1
	}

	if epoch1 == epoch2 {
		// TODO: Razzer's requirement 2. It's wrong that two epochs
		// are same. We can't do threading it more.
		log.Fatalf("wrong racing calls: same epoch")
	}

	for _, c := range p.Calls {
		if c.Thread != 0 {
			// TODO: Razzer's requirment 3. It needs that all syscalls
			// were executed in thread 0
			log.Fatalf("wrong thread: call=%v thread=%d", c.Meta.Name, c.Thread)
		}
	}

	for i := idx1 + 1; i < len(p.Calls); i++ {
		p.Calls[i].Epoch--
		p.Calls[i].Thread = 1
	}
	p.Calls[idx1].Epoch = p.Calls[idx2].Epoch
}
