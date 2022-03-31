package prog

import (
	"math"
	"math/rand"

	"github.com/google/syzkaller/pkg/primitive"
	"github.com/google/syzkaller/pkg/scheduler"
	"github.com/google/syzkaller/pkg/signal"
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
			Point{call: c, addr: ^uint64(0), order: uint64(order + n)})
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
		if pnt.addr != ^uint64(0) {
			break
		}
	}
	p.Schedule.points = p.Schedule.points[:i+1]
}

func (p *Prog) MutateScheduleFromHint(rs rand.Source, hint []primitive.Segment) (bool, []primitive.Segment) {
	if len(p.Contenders()) != 2 {
		return false, hint
	}

	if len(hint) == 0 {
		// TODO: We may want to generate random scheduling points
		return false, nil
	}

	orch := scheduler.Orchestrator{Segs: hint}
	selected := orch.SelectHarmoniousKnots()

	scheduler := scheduler.Scheduler{Knots: selected}
	schedule, ok := scheduler.GenerateSchedPoints()
	if !ok {
		return false, hint
	}
	p.applySchedule(schedule)

	return ok, orch.Segs

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

func (p *Prog) applySchedule(schedule []primitive.Access) {
	shapeScheduleFromAccesses(p, schedule)
	p.appendDummyPoints()
}

func (sched *Schedule) AttachScheduleFilter(filter []uint32) {
	sched.filter = append([]uint32{}, filter...)
}

func (sched Schedule) Filter() []uint32 {
	return sched.filter
}

type randScheduler struct {
	p          *Prog
	r          *randGen
	maxPoints  int
	minPoints  int
	readfrom   signal.ReadFrom
	serial     primitive.SerialAccess
	staleCount map[uint32]int
	candidate  []uint32
	selected   map[uint32]struct{}
	// schedule
	schedule primitive.SerialAccess
	mutated  bool
}

func (ctx *randScheduler) initialize() {
	ctx.candidate = ctx.readfrom.Flatting()
	// TODO: inefficient. need refactoring
	for _, point := range ctx.p.Schedule.points {
		acc, ok := ctx.findAccess(point)
		if !ok {
			continue
		}
		ctx.schedule.Add(acc)
		ctx.selected[acc.Inst] = struct{}{}
	}
	ctx.p.removeDummyPoints()
}

func (ctx *randScheduler) findAccess(point Point) (found primitive.Access, ok bool) {
	// TODO: inefficient. need refactoring
	for _, acc := range ctx.serial {
		if acc.Inst == uint32(point.addr) && acc.Thread == point.call.Thread {
			found, ok = acc, true
			return
		}
	}
	ok = false
	return
}

func (ctx *randScheduler) addPoint() {
	if len(ctx.candidate) == 0 {
		// we don't have any candidate point
		return
	}
	// TODO: IMPORTANT. The logic below is broken. We want to choose a
	// thread along with an instruction. Fix this ASAP.
	for try := 0; try < 10 && ctx.p.Schedule.Len() < ctx.maxPoints; try++ {
		idx := ctx.r.Intn(len(ctx.candidate))
		inst := ctx.candidate[idx]
		if _, selected := ctx.selected[inst]; !selected && !ctx.overused(inst) {
			ctx.makePoint(inst)
			ctx.mutated = true
			break
		}
	}
}

func (ctx *randScheduler) makePoint(inst uint32) {
	// We may have multiple Accesses executing inst. Select any of
	// them.
	accesses := ctx.serial.FindForeachThread(inst, 1)
	if len(accesses) == 0 {
		// TODO: something wrong in this case.
		return
	}
	idx := ctx.r.Intn(len(accesses))
	acc := accesses[idx]
	ctx.schedule.Add(acc)
	ctx.selected[acc.Inst] = struct{}{}
}

func (ctx *randScheduler) overused(addr uint32) bool {
	// y=exp^(-(x^2) / 60pi)
	x := ctx.staleCount[addr]
	prob := math.Exp(float64(x*x*-1) / (60 * math.Pi))
	probInt := int(prob * 1000)
	if probInt == 0 {
		probInt = 1
	}
	var overused bool
	if probInt == 1000 {
		overused = false
	} else {
		overused = !ctx.r.nOutOf(probInt, 1000)
	}
	return overused
}

func (ctx *randScheduler) movePoint() {
	// TODO: Is this really helpful? Why not just remove a point and
	// then add another one?
	if len(ctx.schedule) == 0 {
		// We don't have any scheduling point. Just add a random
		// point.
		ctx.addPoint()
		return
	}
	idx := ctx.r.Intn(len(ctx.schedule))
	// Inclusive range of the new scheduling point
	lower, upper := 0, len(ctx.serial)-1
	if idx != 0 {
		prev := ctx.schedule[idx-1]
		lower = ctx.serial.FindIndex(prev) + 1
	}
	if idx != len(ctx.schedule)-1 {
		next := ctx.schedule[idx+1]
		upper = ctx.serial.FindIndex(next) - 1
	}
	if (upper - lower + 1) <= 0 {
		// XXX: This should not happen. I observed the this once, but
		// cannot reproduce it. To be safe, reset lower and upper (and
		// this is actually fine).
		lower, upper = 0, len(ctx.serial)-1
	}
	selected := ctx.r.Intn(upper-lower+1) + lower
	if selected >= len(ctx.serial) {
		// XXX: I have not observed this. Just to be safe.
		selected = ctx.r.Intn(len(ctx.serial))
	}
	acc0 := ctx.serial[selected]
	ctx.schedule = append(ctx.schedule[:idx], ctx.schedule[idx+1:]...)
	ctx.schedule.Add(acc0)
}

func (ctx *randScheduler) removePoint() {
	if len(ctx.schedule) == 0 {
		return
	}
	idx := ctx.r.Intn(len(ctx.schedule))
	ctx.schedule = append(ctx.schedule[:idx], ctx.schedule[idx+1:]...)
	ctx.mutated = true
}

func (ctx *randScheduler) finalize() {
	// some calls may not have scheduling points. append dummy
	// scheduling points to let QEMU know the execution order of
	// remaining Calls.
	shapeScheduleFromAccesses(ctx.p, ctx.schedule)
	ctx.p.appendDummyPoints()
}

func shapeScheduleFromAccesses(p *Prog, schedule []primitive.Access) {
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
