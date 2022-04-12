package prog

// TODO: We do not use randScheduler for now. I just keep its code
// here in case of we need it again.

// type randScheduler struct {
// 	p          *Prog
// 	r          *randGen
// 	maxPoints  int
// 	minPoints  int
// 	readfrom   signal.ReadFrom
// 	serial     interleaving.SerialAccess
// 	staleCount map[uint32]int
// 	candidate  []uint32
// 	selected   map[uint32]struct{}
// 	// schedule
// 	schedule interleaving.SerialAccess
// 	mutated  bool
// }

// func (ctx *randScheduler) initialize() {
// 	ctx.candidate = ctx.readfrom.Flatting()
// 	// TODO: inefficient. need refactoring
// 	for _, point := range ctx.p.Schedule.points {
// 		acc, ok := ctx.findAccess(point)
// 		if !ok {
// 			continue
// 		}
// 		ctx.schedule.Add(acc)
// 		ctx.selected[acc.Inst] = struct{}{}
// 	}
// 	ctx.p.removeDummyPoints()
// }

// func (ctx *randScheduler) findAccess(point Point) (found interleaving.Access, ok bool) {
// 	// TODO: inefficient. need refactoring
// 	for _, acc := range ctx.serial {
// 		if acc.Inst == uint32(point.addr) && acc.Thread == point.call.Thread {
// 			found, ok = acc, true
// 			return
// 		}
// 	}
// 	ok = false
// 	return
// }

// func (ctx *randScheduler) addPoint() {
// 	if len(ctx.candidate) == 0 {
// 		// we don't have any candidate point
// 		return
// 	}
// 	// TODO: IMPORTANT. The logic below is broken. We want to choose a
// 	// thread along with an instruction. Fix this ASAP.
// 	for try := 0; try < 10 && ctx.p.Schedule.Len() < ctx.maxPoints; try++ {
// 		idx := ctx.r.Intn(len(ctx.candidate))
// 		inst := ctx.candidate[idx]
// 		if _, selected := ctx.selected[inst]; !selected && !ctx.overused(inst) {
// 			ctx.makePoint(inst)
// 			ctx.mutated = true
// 			break
// 		}
// 	}
// }

// func (ctx *randScheduler) makePoint(inst uint32) {
// 	// We may have multiple Accesses executing inst. Select any of
// 	// them.
// 	accesses := ctx.serial.FindForeachThread(inst, 1)
// 	if len(accesses) == 0 {
// 		// TODO: something wrong in this case.
// 		return
// 	}
// 	idx := ctx.r.Intn(len(accesses))
// 	acc := accesses[idx]
// 	ctx.schedule.Add(acc)
// 	ctx.selected[acc.Inst] = struct{}{}
// }

// func (ctx *randScheduler) overused(addr uint32) bool {
// 	// y=exp^(-(x^2) / 60pi)
// 	x := ctx.staleCount[addr]
// 	prob := math.Exp(float64(x*x*-1) / (60 * math.Pi))
// 	probInt := int(prob * 1000)
// 	if probInt == 0 {
// 		probInt = 1
// 	}
// 	var overused bool
// 	if probInt == 1000 {
// 		overused = false
// 	} else {
// 		overused = !ctx.r.nOutOf(probInt, 1000)
// 	}
// 	return overused
// }

// func (ctx *randScheduler) movePoint() {
// 	// TODO: Is this really helpful? Why not just remove a point and
// 	// then add another one?
// 	if len(ctx.schedule) == 0 {
// 		// We don't have any scheduling point. Just add a random
// 		// point.
// 		ctx.addPoint()
// 		return
// 	}
// 	idx := ctx.r.Intn(len(ctx.schedule))
// 	// Inclusive range of the new scheduling point
// 	lower, upper := 0, len(ctx.serial)-1
// 	if idx != 0 {
// 		prev := ctx.schedule[idx-1]
// 		lower = ctx.serial.FindIndex(prev) + 1
// 	}
// 	if idx != len(ctx.schedule)-1 {
// 		next := ctx.schedule[idx+1]
// 		upper = ctx.serial.FindIndex(next) - 1
// 	}
// 	if (upper - lower + 1) <= 0 {
// 		// XXX: This should not happen. I observed the this once, but
// 		// cannot reproduce it. To be safe, reset lower and upper (and
// 		// this is actually fine).
// 		lower, upper = 0, len(ctx.serial)-1
// 	}
// 	selected := ctx.r.Intn(upper-lower+1) + lower
// 	if selected >= len(ctx.serial) {
// 		// XXX: I have not observed this. Just to be safe.
// 		selected = ctx.r.Intn(len(ctx.serial))
// 	}
// 	acc0 := ctx.serial[selected]
// 	ctx.schedule = append(ctx.schedule[:idx], ctx.schedule[idx+1:]...)
// 	ctx.schedule.Add(acc0)
// }

// func (ctx *randScheduler) removePoint() {
// 	if len(ctx.schedule) == 0 {
// 		return
// 	}
// 	idx := ctx.r.Intn(len(ctx.schedule))
// 	ctx.schedule = append(ctx.schedule[:idx], ctx.schedule[idx+1:]...)
// 	ctx.mutated = true
// }

// func (ctx *randScheduler) finalize() {
// 	// some calls may not have scheduling points. append dummy
// 	// scheduling points to let QEMU know the execution order of
// 	// remaining Calls.
// 	shapeScheduleFromAccesses(ctx.p, ctx.schedule)
// 	ctx.p.appendDummyPoints()
// }
