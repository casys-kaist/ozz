package prog

import (
	"math/rand"

	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/scheduler"
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

func (p *Prog) MutateScheduleFromHint(rs rand.Source, hint []interleaving.Segment) (bool, []interleaving.Segment, []interleaving.Segment) {
	if len(p.Contenders()) != 2 {
		return false, nil, hint
	}

	if len(hint) == 0 {
		// TODO: We may want to generate random scheduling points
		return false, nil, nil
	}

	orch := scheduler.Orchestrator{Segs: hint}
	selected := orch.SelectHarmoniousKnots()

	scheduler := scheduler.Scheduler{Knots: selected}
	schedule, ok := scheduler.GenerateSchedPoints()
	if !ok {
		return false, nil, hint
	}
	p.applySchedule(schedule)

	return ok, orch.Used, orch.Segs

	// TODO: Below code can be used to generate a scheduler if we
	// don't have more hints. I don't delete the code (even though it
	// is stored in Git) just in case.

	// r := newRand(p.Target, rs)
	// ctx := &scheduler{
	// 	p:          p,
	// 	r:          r,
	// 	maxPoints:  maxPoints,
	// 	minPoints:  minPoints,
	// 	readfrom:   readfrom,
	// 	staleCount: staleCount,
	// 	selected:   make(map[uint32]struct{}),
	// 	serial:     serial,
	// }
	// ctx.initialize()
	// // If the length of actual scheduling point is 1, try to
	// // mutate more to increase the diversity of interleavings.
	// for stop := false; !stop; stop = r.oneOf(3) || (len(ctx.schedule) < ctx.minPoints && !r.oneOf(5)) {
	// 	switch {
	// 	case r.nOutOf(2, 5): // 40%
	// 		ctx.addPoint()
	// 	case r.nOutOf(5, 6): // 50%
	// 		ctx.movePoint()
	// 	default: // 10%
	// 		ctx.removePoint()
	// 	}
	// }
	// ctx.finalize()
	// return ctx.mutated
}

func (p *Prog) applySchedule(schedule []interleaving.Access) {
	shapeScheduleFromAccesses(p, schedule)
	p.appendDummyPoints()
}

func (sched *Schedule) AttachScheduleFilter(filter []uint32) {
	sched.filter = append([]uint32{}, filter...)
}

func (sched Schedule) Filter() []uint32 {
	return sched.filter
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

const dummyAddr = ^uint64(0)
