package prog

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
)

type Point struct {
	call  *Call
	addr  uint64
	order uint64
}

type Schedule struct {
	points []Point
	// filter[i] == 1: points[i] will not be treated
	// filter[i] == 0: points[i] will be treated
	filter []uint32
}

func (p *Prog) MutateScheduleFromHint(r *rand.Rand, hint interleaving.Hint, randomReordering bool) {
	schedule := hint.GenerateSchedule()
	vec := hint.GenerateFlushVector(r, randomReordering)
	p.applySchedule(schedule)
	p.attachFlushVector(vec)
	p.storeHint(hint)
}

func (p *Prog) attachFlushVector(vec interleaving.FlushVector) {
	p.FlushVector = vec
}

func (p *Prog) storeHint(hint interleaving.Hint) {
	// XXX: This seems bad. I want to keep hint to decide that the
	// fuzzer tested the hint after executing p.
	p.Hint = hint
}

func (p *Prog) appendDummyPoints() {
	if !p.Threaded {
		return
	}
	calls := p.Contenders()
	n := p.Schedule.Len()
	order := 0
	for _, c := range calls {
		if p.Schedule.Match(c).Len() != 0 {
			// c has points
			continue
		}
		p.Schedule.points = append(p.Schedule.points,
			Point{call: c, addr: dummyAddr, order: uint64(order + n)})
		order++
	}
}

func (p *Prog) removeDummyPoints() {
	if !p.Threaded {
		return
	}
	if len(p.Schedule.points) == 0 {
		return
	}
	i := len(p.Schedule.points) - 1
	for ; i >= 0; i-- {
		pnt := p.Schedule.points[i]
		if pnt.addr != dummyAddr {
			break
		}
	}
	p.Schedule.points = p.Schedule.points[:i+1]
}

func (p *Prog) applySchedule(schedule []interleaving.Access) {
	shapeScheduleFromAccesses(p, schedule)
	p.appendDummyPoints()
}

func shapeScheduleFromAccesses(p *Prog, schedule []interleaving.Access) {
	order := uint64(0)
	sched := Schedule{}
	calls := p.Contenders()
	for _, acc := range schedule {
		thread := acc.Thread
		var call *Call
		for _, c := range calls {
			if c.Thread == thread {
				call = c
			}
		}
		if call == nil {
			continue
		}
		sched.points = append(sched.points, Point{
			call:  call,
			addr:  0xffffffff00000000 | uint64(acc.Inst),
			order: order,
		})
		order++
	}
	p.Schedule = sched
}

func (sched Schedule) Len() int {
	return len(sched.points)
}

func (sched Schedule) Match(c *Call) Schedule {
	res := Schedule{}
	for _, point := range sched.points {
		if point.call == c {
			res.points = append(res.points, point)
		}
	}
	return res
}

func (sched Schedule) CallIndex(call *Call, p *Prog) int {
	for ci, c := range p.Calls {
		if c == call {
			return ci
		}
	}
	// something wrong. sched does not have Call.
	return -1
}

func (sched *Schedule) AttachScheduleFilter(filter []uint32) {
	sched.filter = append([]uint32{}, filter...)
}

func (sched Schedule) Filter() []uint32 {
	return sched.filter
}

const dummyAddr = ^uint64(0)
