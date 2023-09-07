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

	knotterOptsPreThreading scheduler.KnotterOpts
	knotterOptsThreading    scheduler.KnotterOpts
	knotterOptsSchedule     scheduler.KnotterOpts

	// To give a half of computing power for scheduling. We don't use
	// proc.fuzzer.Stats and proc.env.StatExec as it is periodically
	// set to 0.
	balancer balancer
	// If scheduled is too large, we block Proc.pickupThreadingWorks()
	// to give more chance to sequential-fuzzing.
	threadingPlugged bool
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

	defaultKnotterOpts := scheduler.KnotterOpts{
		Signal: &fuzzer.maxInterleaving,
		Mu:     &fuzzer.signalMu,
		// In RelRazzer, we want to track/test only parallel knots
		Flags: scheduler.FlagWantParallel,
	}
	knotterOptsPreThreading := defaultKnotterOpts
	knotterOptsPreThreading.Flags |= scheduler.FlagReassignThreadID
	knotterOptsThreading := defaultKnotterOpts
	knotterOptsSchedule := defaultKnotterOpts
	knotterOptsSchedule.Flags |= scheduler.FlagStrictTimestamp

	proc := &Proc{
		fuzzer:                  fuzzer,
		pid:                     pid,
		env:                     env,
		rnd:                     rnd,
		execOpts:                fuzzer.execOpts,
		execOptsCollide:         &execOptsCollide,
		execOptsCover:           &execOptsCover,
		knotterOptsPreThreading: knotterOptsPreThreading,
		knotterOptsThreading:    knotterOptsThreading,
		knotterOptsSchedule:     knotterOptsSchedule,
	}
	return proc, nil
}

func (proc *Proc) loop() {
	generatePeriod := 100
	if proc.fuzzer.config.Flags&ipc.FlagSignal == 0 {
		// If we don't have real coverage signal, generate programs more frequently
		// because fallback signal is weak.
		generatePeriod = 2
	}
	for i := 0; ; i++ {
		proc.powerSchedule(i%100 == 0)

		item := proc.fuzzer.workQueue.dequeue()
		if item != nil {
			switch item := item.(type) {
			case *WorkTriage:
				proc.triageInput(item)
			case *WorkCandidate:
				proc.executeCandidate(item)
			case *WorkSmash:
				proc.smashInput(item)
			case *WorkThreading:
				proc.threadingInput(item)
			default:
				log.Fatalf("unknown work type: %#v", item)
			}
			continue
		}

		ct := proc.fuzzer.choiceTable
		fuzzerSnapshot := proc.fuzzer.snapshot()
		if (len(fuzzerSnapshot.corpus) == 0 || i%generatePeriod == 0) && proc.fuzzer.generate {
			// Generate a new prog.
			p := proc.fuzzer.target.Generate(proc.rnd, prog.RecommendedCalls, ct)
			log.Logf(1, "#%v: generated", proc.pid)
			proc.executeAndCollide(proc.execOpts, p, ProgNormal, StatGenerate)
		} else if i%2 == 1 && proc.fuzzer.generate {
			// Mutate an existing prog.
			p := fuzzerSnapshot.chooseProgram(proc.rnd).Clone()
			p.Mutate(proc.rnd, prog.RecommendedCalls, ct, proc.fuzzer.noMutate, fuzzerSnapshot.corpus)
			log.Logf(1, "#%v: mutated", proc.pid)
			proc.executeAndCollide(proc.execOpts, p, ProgNormal, StatFuzz)
		} else {
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
	for cnt := 0; cnt < 10; cnt++ {
		tp := fuzzerSnapshot.chooseThreadedProgram(proc.rnd)
		if tp == nil {
			break
		}
		p, hint := tp.P.Clone(), proc.pruneHint(tp.Hint)

		cand, used, remaining := scheduler.GenerateCandidates(proc.rnd, hint)
		// We exclude used knots from tp.Hint even if the schedule
		// mutation fails.
		proc.setHint(tp, remaining)
		if !canSchedule(p, cand) {
			// TODO: We may want to generate random scheduling points
			continue
		}
		// NOTE: This may not be necessary, but as we are exploring
		// new research problems, we wanna know what knots are used
		// and understand how the fuzzer can be improved futher.
		proc.inspectUsedKnots(used)

		p.MutateScheduleFromCandidate(proc.rnd, cand)
		p.MutateFlushVectorFromCandidate(proc.rnd, cand, randomReordering)
		// XXX: For easy debugging the kernel
		log.Logf(0, "crit comm: %v --> %v", cand.CriticalComm.Former(), cand.CriticalComm.Latter())
		log.Logf(0, "some inst1: %v", cand.DelayingInst)
		log.Logf(0, "some inst2: %v", cand.SomeInst)

		log.Logf(1, "proc #%v: scheduling an input", proc.pid)
		proc.execute(proc.execOptsCollide, p, ProgNormal, StatSchedule)
		if !proc.needScheduling() {
			break
		}
	}
}

func canSchedule(p *prog.Prog, cand interleaving.Candidate) bool {
	return len(p.Contenders()) == 2 && !cand.Invalid()
}

func (proc *Proc) pruneHint(hint []interleaving.Segment) []interleaving.Segment {
	pruned := make([]interleaving.Segment, 0, len(hint))
	for _, h := range hint {
		hsh := h.Hash()
		if _, ok := proc.fuzzer.corpusInterleaving[hsh]; !ok {
			pruned = append(pruned, h)
		}
	}
	return pruned
}

func (proc *Proc) setHint(tp *prog.ConcurrentCalls, remaining []interleaving.Segment) {
	debugHint(tp, remaining)
	used := len(tp.Hint) - len(remaining)
	proc.fuzzer.subCollection(CollectionScheduleHint, uint64(used))
	proc.fuzzer.corpusMu.Lock()
	defer proc.fuzzer.corpusMu.Unlock()
	tp.Hint = remaining
}

func (proc *Proc) inspectUsedKnots(used []interleaving.Segment) {
	proc.sendUsedKnots(used)
	proc.countUsedInstructions(used)
}

func (proc *Proc) sendUsedKnots(used []interleaving.Segment) {
	// XXX: This may degrade the runtime performance as it keeps
	// invoking RPC calls. Not sure we want to keep this.
	proc.fuzzer.sendUsedKnots(used)
}

func (proc *Proc) countUsedInstructions(used []interleaving.Segment) {
	proc.fuzzer.signalMu.Lock()
	defer proc.fuzzer.signalMu.Unlock()
	for _, _knot := range used {
		knot := _knot.(interleaving.Knot)
		proc.fuzzer.countInstructionInKnot(knot)
	}
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

	if item.flags&ProgSmashed == 0 && proc.fuzzer.generate {
		proc.fuzzer.workQueue.enqueue(&WorkSmash{item.p, item.call})
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

	proc.fuzzer.subCollection(CollectionThreadingHint, uint64(len(item.knots)))

	p := item.p.Clone()
	p.Threading(item.calls)

	knots := proc.executeThreading(p)
	if len(knots) == 0 {
		return
	}

	// newly found knots during threading work
	newKnots := proc.fuzzer.getNewKnot(knots)
	// knots that actually occurred among speculated knots
	speculatedKnots := interleaving.Intersect(knots, item.knots)

	// schedule hint := {newly found knots during threading work}
	// \cup {speculated knots when picking up threading work}
	scheduleHint := append(newKnots, speculatedKnots...)
	if len(scheduleHint) == 0 {
		return
	}
	proc.fuzzer.bookScheduleGuide(p, scheduleHint)
}

func (proc *Proc) executeThreading(p *prog.Prog) []interleaving.Segment {
	hints := []interleaving.Segment{}
	for i := 0; i < 2; i++ {
		inf := proc.executeRaw(proc.execOpts, p, StatThreading)
		seq := proc.sequentialAccesses(inf, p.Contender)
		hints = append(hints, scheduler.ComputeHints(seq)...)
		p.Reverse()
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
	proc.pickupThreadingWorks(p, info)
	return info
}

func (proc *Proc) pickupThreadingWorks(p *prog.Prog, info *ipc.ProgInfo) {
	if proc.threadingPlugged {
		return
	}

	notTooFar := func(c1, c2 int) bool {
		maxIntermediateCalls := 5
		dist := (c2 - c1 - 1)
		return dist < maxIntermediateCalls
	}
	for c1 := 0; c1 < len(p.Calls); c1++ {
		for c2 := c1 + 1; c2 < len(p.Calls) && notTooFar(c1, c2); c2++ {
			if proc.fuzzer.shutOffThreading(p) {
				return
			}
			cont := prog.Contender{Calls: []int{c1, c2}}
			seq := proc.sequentialAccesses(info, cont)
			hints := scheduler.ComputeHints0(seq)
			if len(hints) == 0 {
				continue
			}
			if newHints := proc.fuzzer.getNewKnot(hints); len(newHints) != 0 {
				proc.enqueueThreading(p, cont, newHints)
			}
		}
	}
}

func (proc *Proc) postExecuteThreaded(p *prog.Prog, info *ipc.ProgInfo) *ipc.ProgInfo {
	// NOTE: The scheduling work is the only case reaching here
	knots := proc.extractKnots(info, p.Contender, proc.knotterOptsSchedule)
	if len(knots) == 0 {
		log.Logf(1, "Failed to add sequential traces")
		return info
	}

	if new := proc.fuzzer.newSegment(&proc.fuzzer.corpusInterleaving, knots); len(new) == 0 {
		return info
	}

	cover := interleaving.Cover(knots)
	signal := interleaving.FromCoverToSignal(cover)

	data := p.Serialize()
	log.Logf(2, "added new scheduled input to corpus:\n%s", data)
	proc.fuzzer.sendScheduledInputToManager(rpctype.ScheduledInput{
		Prog:   p.Serialize(),
		Cover:  cover.Serialize(),
		Signal: signal.Serialize(),
	})
	proc.fuzzer.addThreadedInputToCorpus(p, signal)
	return info
}

func (proc *Proc) extractHints(info *ipc.ProgInfo, calls prog.Contender, opts scheduler.KnotterOpts) []interleaving.Segment {
	knotter := scheduler.GetKnotter(opts)

	seq := proc.sequentialAccesses(info, calls)
	if !knotter.AddSequentialTrace(seq) {
		return nil
	}
	knotter.ExcavateKnots()

	return knotter.GetKnots()
}

func (proc *Proc) extractKnots(info *ipc.ProgInfo, calls prog.Contender, opts scheduler.KnotterOpts) []interleaving.Segment {
	// TODO: IMPLEMENT
	return nil
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

func (proc *Proc) enqueueThreading(p *prog.Prog, calls prog.Contender, knots []interleaving.Segment) {
	proc.fuzzer.addCollection(CollectionThreadingHint, uint64(len(knots)))
	proc.fuzzer.workQueue.enqueue(&WorkThreading{
		p:     p.Clone(),
		calls: calls,
		knots: knots,
	})
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
