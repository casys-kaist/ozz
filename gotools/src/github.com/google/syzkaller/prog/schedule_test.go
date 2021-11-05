package prog

import (
	"testing"

	"github.com/google/syzkaller/pkg/signal"
)

func TestMatch(t *testing.T) {
	c0 := &Call{Thread: 0}
	c1 := &Call{Thread: 1}
	c2 := &Call{Thread: 2}
	sched := Schedule{
		points: []Point{
			{call: c0, addr: 0x1, order: 0},
			{call: c1, addr: 0x2, order: 1},
			{call: c0, addr: 0x3, order: 2},
			{call: c2, addr: 0x4, order: 3},
		},
	}
	m0 := sched.Match(c0)
	if l := m0.Len(); l != 2 {
		t.Errorf("wrong length of c0, expected 2, got %d", l)
	}
	m1 := sched.Match(c1)
	if l := m1.Len(); l != 1 {
		t.Errorf("wrong length of c1, expected 1, got %d", l)
	}
	m2 := sched.Match(c2)
	if l := m2.Len(); l != 1 {
		t.Errorf("wrong length of c2, expected 1, got %d", l)
	}
}

func TestAppendDummyPoints(t *testing.T) {
	c0 := &Call{Thread: 0, Epoch: 0}
	c1 := &Call{Thread: 1, Epoch: 1}
	c2 := &Call{Thread: 0, Epoch: 1}
	point := Point{call: c1, order: 0, addr: 0}
	p := &Prog{
		Calls:    []*Call{c0, c1, c2},
		Threaded: true,
		Schedule: Schedule{
			points: []Point{point},
		},
		Contender: Contender{[]int{1, 2}},
	}
	p.appendDummyPoints()
	if len(p.Schedule.points) != 2 {
		t.Errorf("wrong length: %d", len(p.Schedule.points))
	}
	if p.Schedule.points[0] != point {
		t.Errorf("point0 is modified: %v", p.Schedule.points[0])
	}
	p2 := Point{call: c2, order: 1, addr: ^uint64(0)}
	if p.Schedule.points[1] != p2 {
		t.Errorf("point1 is not dummy: %v", p.Schedule.points[1])
	}
}

func TestScheduleFromAccesses(t *testing.T) {
	c0 := &Call{Thread: 0, Epoch: 0}
	c1 := &Call{Thread: 1, Epoch: 1}
	c2 := &Call{Thread: 0, Epoch: 1}
	p := &Prog{
		Calls:     []*Call{c0, c1, c2},
		Threaded:  true,
		Contender: Contender{[]int{1, 2}},
	}
	serial := signal.SerialAccess{}
	serial.Add(signal.NewAccess(0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0))
	serial.Add(signal.NewAccess(0x1, 0x1, 0x0, 0x0, 0x1, 0x0, 0x0))
	serial.Add(signal.NewAccess(0x2, 0x2, 0x0, 0x0, 0x2, 0x1, 0x0))
	serial.Add(signal.NewAccess(0x3, 0x3, 0x0, 0x0, 0x3, 0x0, 0x0))
	serial.Add(signal.NewAccess(0x4, 0x4, 0x0, 0x0, 0x4, 0x1, 0x0))
	p.scheduleFromAccesses(serial)
	if p.Schedule.Len() != 4 {
		t.Errorf("wrong length, got %v", p.Schedule.Len())
	}
	ans := []Point{
		{c2, 0xffffffff00000000, 0x0},
		{c1, 0xffffffff00000002, 0x1},
		{c2, 0xffffffff00000003, 0x2},
		{c1, 0xffffffff00000004, 0x3},
	}
	for i := 0; i < 4; i++ {
		if p.Schedule.points[i] != ans[i] {
			t.Errorf("wrong, %v, %v", p.Schedule.points[i], ans[i])
		}
	}
}
