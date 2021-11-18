package prog

import (
	"math"
	"math/rand"

	"github.com/google/syzkaller/pkg/signal"
)

type Point struct {
	call  *Call
	addr  uint64
	order uint64
}

type Schedule struct {
	points []Point
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
	i := len(p.Schedule.points) - 1
	for ; i >= 0; i-- {
		pnt := p.Schedule.points[i]
		if pnt.addr != ^uint64(0) {
			break
		}
	}
	p.Schedule.points = p.Schedule.points[:i+1]
}

func (p *Prog) MutateSchedule(rs rand.Source, staleCount map[uint32]int, nPoints int, readfrom signal.ReadFrom, serial signal.SerialAccess) bool {
	if len(p.Contenders()) != 2 {
		return false
	}
	r := newRand(p.Target, rs)
	ctx := &scheduler{
		p:          p,
		r:          r,
		nPoints:    nPoints,
		readfrom:   readfrom,
		staleCount: staleCount,
		selected:   make(map[uint32]struct{}),
		serial:     serial,
	}
	ctx.initialize()
	for stop := false; !stop; stop = r.oneOf(3) {
		switch {
		case r.nOutOf(2, 5): // 40%
			ctx.addPoint()
		case r.nOutOf(5, 6): // 50%
			ctx.movePoint()
		default: // 10%
			ctx.removePoint()
		}
	}
	ctx.finalize()
	return ctx.mutated
}

type scheduler struct {
	p          *Prog
	r          *randGen
	nPoints    int
	readfrom   signal.ReadFrom
	serial     signal.SerialAccess
	staleCount map[uint32]int
	candidate  []uint32
	selected   map[uint32]struct{}
	// schedule
	schedule signal.SerialAccess
	mutated  bool
}

func (ctx *scheduler) initialize() {
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

func (ctx *scheduler) findAccess(point Point) (found signal.Access, ok bool) {
	// TODO: inefficient. need refactoring
	for _, acc := range ctx.serial {
		if acc.Owned(point.addr, point.call.Thread) {
			found, ok = acc, true
			return
		}
	}
	ok = false
	return
}

func (ctx *scheduler) addPoint() {
	if len(ctx.candidate) == 0 {
		// we don't have any candidate point
		return
	}
	for try := 0; try < 10 && ctx.p.Schedule.Len() < ctx.nPoints; try++ {
		idx := ctx.r.Intn(len(ctx.candidate))
		inst := ctx.candidate[idx]
		if _, selected := ctx.selected[inst]; !selected && !ctx.overused(inst) {
			ctx.makePoint(inst)
			ctx.mutated = true
			break
		}
	}
}

func (ctx *scheduler) makePoint(inst uint32) {
	// We may have multiple Accesses executing inst. Select any of
	// them.
	accesses := ctx.serial.Find(inst, 1)
	if len(accesses) == 0 {
		// TODO: something wrong in this case.
		return
	}
	idx := ctx.r.Intn(len(accesses))
	acc := accesses[idx]
	ctx.schedule.Add(acc)
	ctx.selected[acc.Inst] = struct{}{}
}

func (ctx *scheduler) overused(addr uint32) bool {
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

func (ctx *scheduler) movePoint() {
	// TODO:
}

func (ctx *scheduler) removePoint() {
	if len(ctx.schedule) == 0 {
		return
	}
	idx := ctx.r.Intn(len(ctx.schedule))
	ctx.schedule = append(ctx.schedule[:idx], ctx.schedule[idx+1:]...)
	ctx.mutated = true
}

func (ctx *scheduler) finalize() {
	// some calls may not have scheduling points. append dummy
	// scheduling points to let QEMU know the execution order of
	// remaining Calls.
	ctx.shapeScheduleFromAccesses()
	ctx.p.appendDummyPoints()
}

func (ctx *scheduler) shapeScheduleFromAccesses() {
	prev := ^uint64(0)
	order := uint64(0)
	sched := Schedule{}
	calls := ctx.p.Contenders()
	for _, acc := range ctx.schedule {
		if acc.ExecutedBy(prev) {
			continue
		}
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
		prev = thread
		order++
	}
	ctx.p.Schedule = sched
}
