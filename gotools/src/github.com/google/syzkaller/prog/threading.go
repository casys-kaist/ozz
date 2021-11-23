package prog

import (
	"fmt"

	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/signal"
)

type Contender struct {
	// Calls represents a subset of prog.Calls that will be executed
	// in parallel.
	// TODO: It might be useful when we run multiple sets of
	// prog.Calls in parallel altogether. Fix this after improving
	// Threading(). See TODO's in Threading().
	Calls []int
}

func (c Contender) IsContender(idx int) bool {
	for _, ci := range c.Calls {
		if idx == ci {
			return true
		}
	}
	return false
}

func (p *Prog) Threading(calls Contender) {
	if len(calls.Calls) == 0 {
		return
	}

	// TODO: Current implementation is the Razzer's threading
	// mechanism. I think we can do better. Improve
	// Fuzzer.identifyContender() and this function together.

	if len(calls.Calls) != 2 {
		// TODO: Razzer's requirement 1. Razzer runs only two syscalls
		// in parallel.
		log.Fatalf("wrong racing calls: %d", len(calls.Calls))
	}

	// If p is threaded, let's transform p into a single thread prog
	// before threading again
	p.unthreading()

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

	// TODO: Razzer requirement 4. denote p is already threaded so we
	// don't thread it more. This is possibly a limittation of
	// Razzer. Improve this if possible.
	p.Threaded = true
	p.Contender = calls
	p.appendDummyPoints()
}

func (p *Prog) Contenders() []*Call {
	res := []*Call{}
	for _, ci := range p.Contender.Calls {
		res = append(res, p.Calls[ci])
	}
	return res
}

func (p *Prog) unthreading() {
	for _, c := range p.Calls {
		c.Thread, c.Epoch = 0, 0
	}
	p.Threaded = false
	p.fixupEpoch()
}

type ThreadedProg struct {
	P         *Prog
	ReadFrom  signal.ReadFrom
	Serial    signal.SerialAccess
	Scheduled int
	Prio      int
}

func (p *Prog) sanitizeRazzer() error {
	p.fixupEpoch()
	epoch := make(map[uint64]uint64)
	for i := 0; i < len(p.Calls); i++ {
		c := p.Calls[i]
		if e, ok := epoch[c.Thread]; ok && e >= c.Epoch {
			return fmt.Errorf("wrong epoch in a thread, thread=%v epoch=%v", c.Thread, c.Epoch)
		}
		epoch[c.Thread] = c.Epoch
	}
	if p.Threaded {
		return p.sanitizeRazzerThreaded()
	} else {
		return p.sanitizeRazzerSequential()
	}
}

func (p *Prog) sanitizeRazzerThreaded() error {
	calls := p.Contenders()
	if len(calls) != 2 {
		return fmt.Errorf("wrong number of contenders: %v", len(calls))
	}
	if calls[0].Epoch != calls[1].Epoch {
		return fmt.Errorf("two contenders do not share epoch %v, %v", calls[0].Epoch, calls[1].Epoch)
	}
	var call *Call
	for _, point := range p.Schedule.points {
		if call == point.call {
			return fmt.Errorf("two scheduling points are on the same call: %v", call.Meta.Name)
		}
		call = point.call
	}
	return nil
}

func (p *Prog) sanitizeRazzerSequential() error {
	calls := p.Contenders()
	if len(calls) != 0 {
		return fmt.Errorf("sequential program has contenders")
	}
	if p.Schedule.Len() != 0 {
		return fmt.Errorf("sequential program has schedules")
	}
	used := make(map[uint64]struct{})
	for _, c := range p.Calls {
		if _, ok := used[c.Epoch]; ok {
			return fmt.Errorf("more than one call share epoch %v", c.Epoch)
		}
		used[c.Epoch] = struct{}{}
	}
	return nil
}
