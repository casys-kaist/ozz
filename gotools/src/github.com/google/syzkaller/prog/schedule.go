package prog

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

func sequentialSchedule(p *Prog) Schedule {
	s := Schedule{}
	if !p.Threaded {
		return s
	}
	calls := p.Contenders()
	for i, c := range calls {
		s.points = append(s.points,
			Point{call: c, addr: ^uint64(0), order: uint64(i)})
	}
	return s
}
