// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/google/syzkaller/pkg/cover"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/scheduler"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/prog"
)

// Proc represents a single fuzzing process (executor).
type Proc struct {
	fuzzer          *Fuzzer
	pid             int
	env             *ipc.Env
	rnd             *rand.Rand
	execOpts        *ipc.ExecOpts
	execOptsCollide *ipc.ExecOpts
	execOptsCover   *ipc.ExecOpts

	// To give a half of computing power for scheduling. We don't use
	// proc.fuzzer.Stats and proc.env.StatExec as it is periodically
	// set to 0.
	balancer balancer
}

func newProc(fuzzer *Fuzzer, pid int) (*Proc, error) {
	env, err := ipc.MakeEnv(fuzzer.config, pid)
	if err != nil {
		return nil, err
	}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(pid)*1e12))
	execOptsCollide := *fuzzer.execOpts
	execOptsCollide.Flags |= ipc.FlagCollectCover
	execOptsCollide.Flags &= ^ipc.FlagCollectSignal
	execOptsCollide.Flags |= ipc.FlagTurnOnKSSB
	execOptsCover := *fuzzer.execOpts
	execOptsCover.Flags |= ipc.FlagCollectCover

	proc := &Proc{
		fuzzer:          fuzzer,
		pid:             pid,
		env:             env,
		rnd:             rnd,
		execOpts:        fuzzer.execOpts,
		execOptsCollide: &execOptsCollide,
		execOptsCover:   &execOptsCover,
	}
	return proc, nil
}

func (proc *Proc) startCollectingAccess() {
	proc.execOpts.Flags |= ipc.FlagCollectAccess
	proc.execOptsCollide.Flags |= ipc.FlagCollectAccess
	proc.execOptsCover.Flags |= ipc.FlagCollectAccess
}

func (proc *Proc) loop() {
	generatePeriod := 100
	if proc.fuzzer.config.Flags&ipc.FlagSignal == 0 {
		// If we don't have real coverage signal, generate programs more frequently
		// because fallback signal is weak.
		generatePeriod = 2
	}

	for i := 0; ; i++ {
		proc.fuzzer.m.end()
		proc.powerSchedule()
		item := proc.fuzzer.workQueue.dequeue()
		if item != nil {
			switch item := item.(type) {
			case *WorkTriage:
				proc.fuzzer.m.start(triage)
				proc.triageInput(item)
			case *WorkCandidate:
				proc.fuzzer.m.start(candidate)
				proc.executeCandidate(item)
			case *WorkSmash:
				proc.fuzzer.m.start(smash)
				proc.smashInput(item)
			case *WorkThreading:
				proc.fuzzer.m.start(threading)
				proc.threadingInput(item)
			default:
				log.Fatalf("unknown work type: %#v", item)
			}
			continue
		}

		ct := proc.fuzzer.choiceTable
		fuzzerSnapshot := proc.fuzzer.snapshot()
		if (len(fuzzerSnapshot.corpus) == 0 || i%generatePeriod == 0) && proc.fuzzer.generate {
			proc.fuzzer.m.start(gen)
			// Generate a new prog.
			p := proc.fuzzer.target.Generate(proc.rnd, prog.RecommendedCalls, ct)
			log.Logf(1, "#%v: generated", proc.pid)
			proc.executeAndCollide(proc.execOpts, p, ProgNormal, StatGenerate)
		} else if i%2 == 1 && proc.fuzzer.generate {
			proc.fuzzer.m.start(fuzz)
			// Mutate an existing prog.
			p := fuzzerSnapshot.chooseProgram(proc.rnd).Clone()
			p.Mutate(proc.rnd, prog.RecommendedCalls, ct, proc.fuzzer.noMutate, fuzzerSnapshot.corpus)
			log.Logf(1, "#%v: mutated", proc.pid)
			proc.executeAndCollide(proc.execOpts, p, ProgNormal, StatFuzz)
		} else {
			proc.fuzzer.m.start(schedule)
			// Mutate a schedule of an existing prog.
			proc.scheduleInput(fuzzerSnapshot)
		}
	}
}

func (proc *Proc) scheduleInput(fuzzerSnapshot FuzzerSnapshot) {
	randomReordering := proc.fuzzer.randomReordering
	// NOTE: proc.scheduleInput() does not queue additional works, so
	// executing proc.scheduleInput() does not cause the workqueues
	// exploding.
	for cnt := 0; cnt < 10 && proc.needScheduling(); cnt++ {
		tp := fuzzerSnapshot.chooseThreadedProgram(proc.rnd)
		if tp == nil {
			break
		}
		p, hint := proc.pickHint(tp)
		p.MutateScheduleFromHint(proc.rnd, hint, randomReordering)
		log.Logf(1, "proc #%v: scheduling an input", proc.pid)
		proc.execute(proc.execOptsCollide, p, ProgNormal, StatSchedule)
	}
}

func (proc *Proc) pickHint(tp *prog.ConcurrentCalls) (*prog.Prog, interleaving.Hint) {
retry:
	hints, l := tp.Hint, len(tp.Hint)
	hint := hints[l-1]
	hints = hints[:l-1]
	proc.fuzzer.subCollection(CollectionScheduleHint, 1)
	proc.fuzzer.corpusMu.Lock()
	tp.Hint = hints
	proc.fuzzer.corpusMu.Unlock()
	if hint.Invalid() {
		goto retry
	}
	switch hint.Typ {
	case interleaving.TestingStoreBarrier:
		atomic.AddUint64(&proc.fuzzer.stats[StatTestStoreReordering], 1)
	case interleaving.TestingLoadBarrier:
		atomic.AddUint64(&proc.fuzzer.stats[StatTestLoadReordering], 1)
	}
	if len(tp.Hint) != 0 {
		proc.fuzzer.__bookScheduleGuide(tp)
	} else {
		proc.fuzzer.subCollection(CollectionConcurrentCalls, 1)
	}
	// To debug the kernel easily
	log.Logf(0, "%v", hint)
	return tp.P.Clone(), hint
}

func (proc *Proc) triageInput(item *WorkTriage) {
	log.Logf(1, "#%v: triaging type=%x", proc.pid, item.flags)

	prio := signalPrio(item.p, &item.info, item.call)
	inputSignal := signal.FromRaw(item.info.Signal, prio)
	newSignal := proc.fuzzer.corpusSignalDiff(inputSignal)
	if newSignal.Empty() {
		return
	}
	callName := ".extra"
	logCallName := "extra"
	if item.call != -1 {
		callName = item.p.Calls[item.call].Meta.Name
		logCallName = fmt.Sprintf("call #%v %v", item.call, callName)
	}
	log.Logf(3, "triaging input for %v (new signal=%v)", logCallName, newSignal.Len())
	var inputCover cover.Cover
	const (
		signalRuns       = 3
		minimizeAttempts = 3
	)
	// Compute input coverage and non-flaky signal for minimization.
	notexecuted := 0
	rawCover := []uint32{}
	for i := 0; i < signalRuns; i++ {
		info := proc.executeRaw(proc.execOptsCover, item.p, StatTriage)
		if !reexecutionSuccess(info, &item.info, item.call) {
			// The call was not executed or failed.
			notexecuted++
			if notexecuted > signalRuns/2+1 {
				return // if happens too often, give up
			}
			continue
		}
		thisSignal, thisCover := getSignalAndCover(item.p, info, item.call)
		if len(rawCover) == 0 && proc.fuzzer.fetchRawCover {
			rawCover = append([]uint32{}, thisCover...)
		}
		newSignal = newSignal.Intersection(thisSignal)
		// Without !minimized check manager starts losing some considerable amount
		// of coverage after each restart. Mechanics of this are not completely clear.
		if newSignal.Empty() && item.flags&ProgMinimized == 0 {
			return
		}
		inputCover.Merge(thisCover)
	}
	if item.flags&ProgMinimized == 0 {
		item.p, item.call = prog.Minimize(item.p, item.call, false,
			func(p1 *prog.Prog, call1 int) bool {
				for i := 0; i < minimizeAttempts; i++ {
					info := proc.execute(proc.execOpts, p1, ProgNormal, StatMinimize)
					if !reexecutionSuccess(info, &item.info, call1) {
						// The call was not executed or failed.
						continue
					}
					thisSignal, _ := getSignalAndCover(p1, info, call1)
					if newSignal.Intersection(thisSignal).Len() == newSignal.Len() {
						return true
					}
				}
				return false
			})
	}

	data := item.p.Serialize()
	sig := hash.Hash(data)

	log.Logf(2, "added new input for %v to corpus:\n%s", logCallName, data)
	proc.fuzzer.sendInputToManager(rpctype.Input{
		Call:     callName,
		CallID:   item.call,
		Prog:     data,
		Signal:   inputSignal.Serialize(),
		Cover:    inputCover.Serialize(),
		RawCover: rawCover,
	})

	proc.fuzzer.addInputToCorpus(item.p, inputSignal, sig)

	if item.flags&ProgSmashed == 0 && proc.fuzzer.generate && false {
		proc.fuzzer.workQueue.enqueue(&WorkSmash{item.p, item.call})
		proc.fuzzer.collectionWorkqueue(proc.fuzzer.workQueue.stats())
	}
}

func reexecutionSuccess(info *ipc.ProgInfo, oldInfo *ipc.CallInfo, call int) bool {
	if info == nil || len(info.Calls) == 0 {
		return false
	}
	if call != -1 {
		// Don't minimize calls from successful to unsuccessful.
		// Successful calls are much more valuable.
		if oldInfo.Errno == 0 && info.Calls[call].Errno != 0 {
			return false
		}
		return len(info.Calls[call].Signal) != 0
	}
	return len(info.Extra.Signal) != 0
}

func getSignalAndCover(p *prog.Prog, info *ipc.ProgInfo, call int) (signal.Signal, []uint32) {
	inf := &info.Extra
	if call != -1 {
		inf = &info.Calls[call]
	}
	return signal.FromRaw(inf.Signal, signalPrio(p, inf, call)), inf.Cover
}

func (proc *Proc) executeCandidate(item *WorkCandidate) {
	log.Logf(1, "#%v: executing a candidate", proc.pid)
	proc.execute(proc.execOpts, item.p, item.flags, StatCandidate)
}

func (proc *Proc) smashInput(item *WorkSmash) {
	if proc.fuzzer.faultInjectionEnabled && item.call != -1 {
		proc.failCall(item.p, item.call)
	}
	fuzzerSnapshot := proc.fuzzer.snapshot()
	for i := 0; i < 30; i++ {
		p := item.p.Clone()
		p.Mutate(proc.rnd, prog.RecommendedCalls, proc.fuzzer.choiceTable, proc.fuzzer.noMutate, fuzzerSnapshot.corpus)
		log.Logf(1, "#%v: smash mutated", proc.pid)
		proc.executeAndCollide(proc.execOpts, p, ProgNormal, StatSmash)
	}
}

func (proc *Proc) threadingInput(item *WorkThreading) {
	log.Logf(1, "proc #%v: threading an input", proc.pid)

	proc.fuzzer.subCollection(CollectionThreadingHint, uint64(len(item.hints)))

	p := item.p.Clone()
	p.Threading(item.calls)

	hints := proc.executeThreading(p)
	if len(hints) == 0 {
		return
	}

	prev := proc.fuzzer.m.end()
	proc.fuzzer.m.start(calc2)
	defer func() {
		proc.fuzzer.m.end()
		proc.fuzzer.m.start(prev)
	}()
	// newly found knots during threading work
	newHints := proc.fuzzer.getNewHints(hints)
	// hints that actually occurred among speculated hints
	speculatedHints := interleaving.Select(item.hints, hints)
	scheduleHint := append(newHints, speculatedHints...)
	if len(scheduleHint) == 0 {
		return
	}
	proc.fuzzer.bookScheduleGuide(p, scheduleHint)
}

func (proc *Proc) executeThreading(p *prog.Prog) []interleaving.Hint {
	hints := []interleaving.Hint{}
	for i := 0; i < 2; i++ {
		inf := proc.executeRaw(proc.execOpts, p, StatThreading)
		prev := proc.fuzzer.m.end()
		proc.fuzzer.m.start(calc2)
		seq := proc.sequentialAccesses(inf, p.Contender)
		hints = append(hints, scheduler.ComputeHints(seq)...)
		p.Reverse()
		proc.fuzzer.m.end()
		proc.fuzzer.m.start(prev)
	}
	return hints
}

func (proc *Proc) failCall(p *prog.Prog, call int) {
	for nth := 1; nth <= 100; nth++ {
		log.Logf(1, "#%v: injecting fault into call %v/%v", proc.pid, call, nth)
		newProg := p.Clone()
		newProg.Calls[call].Props.FailNth = nth
		info := proc.executeRaw(proc.execOpts, newProg, StatSmash)
		if info != nil && len(info.Calls) > call && info.Calls[call].Flags&ipc.CallFaultInjected == 0 {
			break
		}
	}
}

func (proc *Proc) execute(execOpts *ipc.ExecOpts, p *prog.Prog, flags ProgTypes, stat Stat) (info *ipc.ProgInfo) {
	info = proc.executeRaw(execOpts, p, stat)
	if info == nil {
		return nil
	}
	defer func() {
		// From this point, all those results will not be used
		for _, c := range info.Calls {
			c.Access = nil
		}
	}()

	if !p.Threaded {
		return proc.postExecute(p, flags, info)
	} else {
		// We run concurrent calls only after triaging corpus
		return proc.postExecuteThreaded(p, info)
	}
}

func (proc *Proc) postExecute(p *prog.Prog, flags ProgTypes, info *ipc.ProgInfo) *ipc.ProgInfo {
	// looking for code coverage
	calls, extra := proc.fuzzer.checkNewSignal(p, info)
	for _, callIndex := range calls {
		proc.enqueueCallTriage(p, flags, callIndex, info.Calls[callIndex])
	}
	if extra {
		proc.enqueueCallTriage(p, flags, -1, info.Extra)
	}
	if proc.fuzzer.schedule {
		proc.pickupThreadingWorks(p, info)
	}
	return info
}

func (proc *Proc) pickupThreadingWorks(p *prog.Prog, info *ipc.ProgInfo) {
	prev := proc.fuzzer.m.end()
	proc.fuzzer.m.start(calc1)
	defer func() {
		proc.fuzzer.m.end()
		proc.fuzzer.m.start(prev)
	}()
	const maxDist = 10
	start := time.Now()
	log.Logf(0, "pick up threading works at %v", start)
	for dist := 1; dist < maxDist; dist++ {
		for c1 := 0; c1 < len(p.Calls) && c1+dist < len(p.Calls); c1++ {
			c2 := c1 + dist
			cont := prog.Contender{Calls: []int{c1, c2}}
			seq := proc.sequentialAccesses(info, cont)
			hints := scheduler.ComputeHints0(seq)
			if len(hints) == 0 {
				continue
			}
			if newHints := proc.fuzzer.getNewHints(hints); len(newHints) != 0 {
				proc.enqueueThreading(p, cont, newHints)
			}
			if time.Since(start) > 10*time.Minute {
				// At this point computing hints can be very slow and
				// I suspect that it can cause "no output from test
				// machine". Stop calculating hints if it takes too
				// long time.
				atomic.AddUint64(&proc.fuzzer.stats[StatThreadWorkTimeout], 1)
				return
			}
		}
	}
}

func (proc *Proc) postExecuteThreaded(p *prog.Prog, info *ipc.ProgInfo) *ipc.ProgInfo {
	// NOTE: The scheduling work is the only case reaching here
	seq := proc.sequentialAccesses(info, p.Contender)
	sign := interleaving.CheckCoverage(seq, p.Hint)
	_ = sign
	return info
}

func (proc *Proc) sequentialAccesses(info *ipc.ProgInfo, calls prog.Contender) (seq []interleaving.SerialAccess) {
	proc.fuzzer.signalMu.RLock()
	for _, call := range calls.Calls {
		serial := interleaving.SerialAccess{}
		for _, acc := range info.Calls[call].Access {
			if _, ok := proc.fuzzer.instBlacklist[acc.Inst]; ok {
				continue
			}
			serial = append(serial, acc)
		}
		seq = append(seq, serial)
	}
	proc.fuzzer.signalMu.RUnlock()
	if len(seq) != 2 {
		// XXX: This is a current implementation's requirement. We
		// need exactly two traces. If info does not contain exactly
		// two traces (e.g., one contender call does not give us its
		// trace), just return nil to let a caller handle this case as
		// an error.
		return nil
	}
	return
}

func (proc *Proc) enqueueCallTriage(p *prog.Prog, flags ProgTypes, callIndex int, info ipc.CallInfo) {
	if !proc.fuzzer.generate {
		// XXX: fuzzer.generate is mostly for debugging, and if we
		// turn off generate, triage is also pretty meaningless.
		return
	}

	// info.Signal points to the output shmem region, detach it before queueing.
	info.Signal = append([]uint32{}, info.Signal...)
	// None of the caller use Cover, so just nil it instead of detaching.
	// Note: triage input uses executeRaw to get coverage.
	info.Cover = nil
	proc.fuzzer.workQueue.enqueue(&WorkTriage{
		p:     p.Clone(),
		call:  callIndex,
		info:  info,
		flags: flags,
	})
	proc.fuzzer.collectionWorkqueue(proc.fuzzer.workQueue.stats())
}

func (proc *Proc) executeAndCollide(execOpts *ipc.ExecOpts, p *prog.Prog, flags ProgTypes, stat Stat) {
	proc.execute(execOpts, p, flags, stat)
	// We do not want to run programs in the collide mode
	return
}

func (proc *Proc) randomCollide(origP *prog.Prog) *prog.Prog {
	// Old-styl collide with a 33% probability.
	if proc.rnd.Intn(3) == 0 {
		p, err := prog.DoubleExecCollide(origP, proc.rnd)
		if err == nil {
			return p
		}
	}
	p := prog.AssignRandomAsync(origP, proc.rnd)
	if proc.rnd.Intn(2) != 0 {
		prog.AssignRandomRerun(p, proc.rnd)
	}
	return p
}

func (proc *Proc) enqueueThreading(p *prog.Prog, calls prog.Contender, hints []interleaving.Hint) {
	proc.fuzzer.addCollection(CollectionThreadingHint, uint64(len(hints)))
	proc.fuzzer.workQueue.enqueue(&WorkThreading{
		p:     p.Clone(),
		calls: calls,
		hints: hints,
	})
	proc.fuzzer.collectionWorkqueue(proc.fuzzer.workQueue.stats())
}

func (proc *Proc) executeRaw(opts *ipc.ExecOpts, p *prog.Prog, stat Stat) *ipc.ProgInfo {
	proc.balancer.count(stat)
	proc.fuzzer.checkDisabledCalls(p)

	// Limit concurrency window and do leak checking once in a while.
	ticket := proc.fuzzer.gate.Enter()
	defer proc.fuzzer.gate.Leave(ticket)

	proc.logProgram(opts, p)
	for try := 0; ; try++ {
		atomic.AddUint64(&proc.fuzzer.stats[stat], 1)
		output, info, hanged, err := proc.env.Exec(opts, p)
		if err != nil {
			if err == prog.ErrExecBufferTooSmall {
				// It's bad if we systematically fail to serialize programs,
				// but so far we don't have a better handling than counting this.
				// This error is observed a lot on the seeded syz_mount_image calls.
				atomic.AddUint64(&proc.fuzzer.stats[StatBufferTooSmall], 1)
				return nil
			}
			if try > 10 {
				log.Fatalf("executor %v failed %v times: %v", proc.pid, try, err)
			}
			log.Logf(4, "fuzzer detected executor failure='%v', retrying #%d", err, try+1)
			debug.FreeOSMemory()
			time.Sleep(time.Second)
			continue
		}

		proc.shiftAccesses(info)

		retry := needRetry(p, info)
		log.Logf(2, "result hanged=%v retry=%v: %s", hanged, retry, output)
		if retry {
			filter := buildScheduleFilter(p, info)
			p.AttachScheduleFilter(filter)
			if try > 10 {
				log.Logf(2, "QEMU/executor require too many retries. Ignore")
				return info
			}
			continue
		}
		return info
	}
}

func (proc *Proc) shiftAccesses(info *ipc.ProgInfo) {
	if proc.fuzzer.shifter == nil {
		return
	}
	for i := range info.Calls {
		for j := range info.Calls[i].Access {
			inst := info.Calls[i].Access[j].Inst
			if shift, ok := proc.fuzzer.shifter[inst]; ok {
				info.Calls[i].Access[j].Inst += shift
			}
		}
	}
}

func needRetry(p *prog.Prog, info *ipc.ProgInfo) bool {
	retry := false
	for _, ci := range p.Contender.Calls {
		inf := info.Calls[ci]
		if inf.Flags&ipc.CallRetry != 0 {
			retry = true
			break
		}
	}
	return retry
}

func buildScheduleFilter(p *prog.Prog, info *ipc.ProgInfo) []uint32 {
	const FOOTPRINT_MISSED = 1
	filter := make([]uint32, p.Schedule.Len())
	for _, ci := range info.Calls {
		for _, outcome := range ci.SchedpointOutcome {
			order := outcome.Order
			if order >= uint32(len(filter)) {
				return nil
			}
			if outcome.Footprint == FOOTPRINT_MISSED {
				filter[order] = 1
			}
		}
	}
	return filter
}
