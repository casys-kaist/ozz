// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/syzkaller/pkg/affinity"
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
	fuzzer            *Fuzzer
	pid               int
	env               *ipc.Env
	rnd               *rand.Rand
	execOpts          *ipc.ExecOpts
	execOptsCover     *ipc.ExecOpts
	execOptsComps     *ipc.ExecOpts
	execOptsNoCollide *ipc.ExecOpts

	knotterOptsThreading knotterOpts
	knotterOptsSchedule  knotterOpts

	// To give a half of computing power for scheduling. We don't use
	// proc.fuzzer.Stats and proc.env.StatExec as it is periodically
	// set to 0.
	executed  uint64
	scheduled uint64
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
	execOptsNoCollide := *fuzzer.execOpts
	execOptsNoCollide.Flags &= ^ipc.FlagCollide
	execOptsCover := execOptsNoCollide
	execOptsCover.Flags |= ipc.FlagCollectCover
	execOptsComps := execOptsNoCollide
	execOptsComps.Flags |= ipc.FlagCollectComps
	knotterOptsThreading := knotterOpts{&fuzzer.maxInterleaving, true, false}
	knotterOptsSchedule := knotterOpts{&fuzzer.corpusInterleaving, false, true}
	proc := &Proc{
		fuzzer:               fuzzer,
		pid:                  pid,
		env:                  env,
		rnd:                  rnd,
		execOpts:             fuzzer.execOpts,
		execOptsCover:        &execOptsCover,
		execOptsComps:        &execOptsComps,
		execOptsNoCollide:    &execOptsNoCollide,
		knotterOptsThreading: knotterOptsThreading,
		knotterOptsSchedule:  knotterOptsSchedule,
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
		log.Logf(2, "executed=%v scheduled=%v", proc.executed, proc.scheduled)
		proc.relieveMemoryPressure()
		if i%100 == 0 {
			proc.powerSchedule()
		}

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
			log.Logf(1, "proc #%v: generated", proc.pid)
			proc.execute(proc.execOpts, p, ProgNormal, StatGenerate)
		} else if i%2 == 1 && proc.fuzzer.generate {
			// Mutate an existing prog.
			p := fuzzerSnapshot.chooseProgram(proc.rnd).Clone()
			p.Mutate(proc.rnd, prog.RecommendedCalls, ct, fuzzerSnapshot.corpus)
			log.Logf(1, "proc #%v: mutated", proc.pid)
			proc.execute(proc.execOpts, p, ProgNormal, StatFuzz)
		} else {
			// Mutate a schedule of an existing prog.
			proc.scheduleInput(fuzzerSnapshot)
		}
		if i%100 == 0 && !affinity.RunOnCPU(1<<0) {
			log.Logf(0, "[WARN] Proc goroutine #%v runs on CPU other than 0", proc.pid)
		}
	}
}

func (proc *Proc) powerSchedule() {
	if proc.threadingPlugged {
		proc.unplugThreading()
	} else {
		proc.plugThreading()
	}
}

func (proc *Proc) unplugThreading() {
	if proc.scheduled < uint64(float64(proc.executed)*0.4) {
		proc.fuzzer.addCollection(CollectionUnplug, 1)
		proc.threadingPlugged = false
	}
}

func (proc *Proc) plugThreading() {
	if proc.scheduled > uint64(float64(proc.executed)*0.7) {
		proc.fuzzer.addCollection(CollectionPlug, 1)
		proc.threadingPlugged = true
	}
}

func (proc *Proc) relieveMemoryPressure() {
	needSchedule := proc.fuzzer.spillScheduling()
	needThreading := proc.fuzzer.spillThreading()
	if !needSchedule && !needThreading {
		return
	}
	MonitorMemUsage()
	for cnt := 0; (needSchedule || needThreading) && cnt < 10; cnt++ {
		log.Logf(2, "Relieving memory pressure schedule=%v threading=%v", needSchedule, needThreading)
		if needSchedule {
			fuzzerSnapshot := proc.fuzzer.snapshot()
			proc.scheduleInput(fuzzerSnapshot)
		} else if item := proc.fuzzer.workQueue.dequeueThreading(); item != nil {
			proc.threadingInput(item)
		}
		needSchedule = proc.fuzzer.spillScheduling()
		needThreading = proc.fuzzer.spillThreading()
		if !needSchedule && !needThreading {
			break
		}
	}
	return
}

func (proc *Proc) needScheduling() bool {
	if len(proc.fuzzer.candidates) == 0 {
		return false
	}

	// prob = 1 / (1 + exp(-25 * (-x + 0.25))) where x = (scheduled/executed)
	x := float64(proc.scheduled) / float64(proc.executed)
	prob1000 := int(1 / (1 + math.Exp(-30*(-1*x+0.25))) * 1000)
	if prob1000 < 50 {
		prob1000 = 50
	}
	return prob1000 >= proc.rnd.Intn(1000)
}

func (proc *Proc) scheduleInput(fuzzerSnapshot FuzzerSnapshot) {
	// NOTE: proc.scheduleInput() does not queue additional works, so
	// executing proc.scheduleInput() does not cause the workqueues
	// exploding.
	for cnt := 0; cnt < 10; cnt++ {
		tp := fuzzerSnapshot.chooseThreadedProgram(proc.rnd)
		if tp == nil {
			break
		}
		p, hint := tp.P.Clone(), tp.Hint

		ok, remaining := p.MutateScheduleFromHint(proc.rnd, hint)
		proc.setHint(tp, remaining)
		// We exclude used knots from tp.Hint even if the schedule
		// mutation fails.
		if !ok {
			continue
		}

		log.Logf(1, "proc #%v: scheduling an input", proc.pid)
		proc.scheduled++
		proc.execute(proc.execOpts, p, ProgNormal, StatSchedule)
		if !proc.needScheduling() {
			break
		}
	}
}

func (proc *Proc) setHint(tp *prog.Candidate, remaining []interleaving.Segment) {
	debugHint(tp, remaining)
	used := len(tp.Hint) - len(remaining)
	proc.fuzzer.subCollection(CollectionScheduleHint, uint64(used))
	proc.fuzzer.corpusMu.Lock()
	defer proc.fuzzer.corpusMu.Unlock()
	tp.Hint = remaining
}

func (proc *Proc) triageInput(item *WorkTriage) {
	log.Logf(1, "proc #%v: triaging type=%x", proc.pid, item.flags)

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
					info := proc.execute(proc.execOptsNoCollide, p1, ProgNormal, StatMinimize)
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
	proc.fuzzer.sendInputToManager(rpctype.RPCInput{
		Call:   callName,
		Prog:   data,
		Signal: inputSignal.Serialize(),
		Cover:  inputCover.Serialize(),
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
	log.Logf(1, "proc #%v: executing a candidate", proc.pid)
	proc.execute(proc.execOpts, item.p, item.flags, StatCandidate)
}

func (proc *Proc) smashInput(item *WorkSmash) {
	if proc.fuzzer.faultInjectionEnabled && item.call != -1 {
		proc.failCall(item.p, item.call)
	}
	if proc.fuzzer.comparisonTracingEnabled && item.call != -1 {
		proc.executeHintSeed(item.p, item.call)
	}
	fuzzerSnapshot := proc.fuzzer.snapshot()
	for i := 0; i < 30; i++ {
		p := item.p.Clone()
		p.Mutate(proc.rnd, prog.RecommendedCalls, proc.fuzzer.choiceTable, fuzzerSnapshot.corpus)
		log.Logf(1, "proc #%v: smash mutated", proc.pid)
		proc.execute(proc.execOpts, p, ProgNormal, StatSmash)
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
	newKnots := proc.fuzzer.newKnot(knots)
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
	knotter := scheduler.GetKnotter(&proc.fuzzer.maxInterleaving, &proc.fuzzer.signalMu)
	for i := 0; i < 2; i++ {
		inf := proc.executeRaw(proc.execOpts, p, StatThreading)
		seq := sequentialTrace(inf)
		if !knotter.AddSequentialTrace(seq) {
			log.Logf(1, "Failed to add sequential traces")
			return nil
		}
		p.Reverse()
	}
	knotter.ExcavateKnots()
	return knotter.GetKnots()
}

func sequentialTrace(info *ipc.ProgInfo) []interleaving.SerialAccess {
	// TODO: This should be called with a threaded Prog
	res := []interleaving.SerialAccess{}
	for _, c := range info.Calls {
		if len(c.Access) != 0 {
			res = append(res, c.Access)
		}
	}
	if len(res) != 2 {
		// XXX: This is a current implementation's requirement. We
		// need exactly two traces. If info does not contain exactly
		// two traces (e.g., one contender call does not give us its
		// trace), just return nil to let a caller handle this case as
		// an error.
		return nil
	}
	return res
}

func (proc *Proc) failCall(p *prog.Prog, call int) {
	for nth := 0; nth < 100; nth++ {
		log.Logf(1, "proc #%v: injecting fault into call %v/%v", proc.pid, call, nth)
		opts := *proc.execOpts
		opts.Flags |= ipc.FlagInjectFault
		opts.FaultCall = call
		opts.FaultNth = nth
		info := proc.executeRaw(&opts, p, StatSmash)
		if info != nil && len(info.Calls) > call && info.Calls[call].Flags&ipc.CallFaultInjected == 0 {
			break
		}
	}
}

func (proc *Proc) executeHintSeed(p *prog.Prog, call int) {
	log.Logf(1, "proc #%v: collecting comparisons", proc.pid)
	// First execute the original program to dump comparisons from KCOV.
	info := proc.execute(proc.execOptsComps, p, ProgNormal, StatSeed)
	if info == nil {
		return
	}

	// Then mutate the initial program for every match between
	// a syscall argument and a comparison operand.
	// Execute each of such mutants to check if it gives new coverage.
	p.MutateWithHints(call, info.Calls[call].Comps, func(p *prog.Prog) {
		log.Logf(1, "proc #%v: executing comparison hint", proc.pid)
		proc.execute(proc.execOpts, p, ProgNormal, StatHint)
	})
}

func (proc *Proc) execute(execOpts *ipc.ExecOpts, p *prog.Prog, flags ProgTypes, stat Stat) (info *ipc.ProgInfo) {
	info = proc.executeRaw(execOpts, p, stat)
	if info == nil {
		return
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

	maxIntermediateCalls := 2
	intermediateCalls := func(c1, c2 int) int {
		return c2 - c1 - 1
	}
	for c1 := 0; c1 < len(p.Calls); c1++ {
		for c2 := c1 + 1; c2 < len(p.Calls) && intermediateCalls(c1, c2) < maxIntermediateCalls; c2++ {
			if proc.fuzzer.shutOffThreading(p) {
				return
			}

			cont := prog.Contender{Calls: []int{c1, c2}}
			knots := proc.extractKnots(info, cont, proc.knotterOptsThreading)
			if len(knots) == 0 {
				continue
			}
			if newKnots := proc.fuzzer.newKnot(knots); len(newKnots) != 0 {
				proc.enqueueThreading(p, cont, newKnots)
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
	proc.fuzzer.sendScheduledInputToManager(rpctype.RPCScheduledInput{
		Prog:   p.Serialize(),
		Cover:  cover.Serialize(),
		Signal: signal.Serialize(),
	})
	proc.fuzzer.addThreadedInputToCorpus(p, signal)
	return info
}

type knotterOpts struct {
	collected        *interleaving.Signal
	reassignThreadID bool
	strictTimestamp  bool
}

func (proc *Proc) extractKnots(info *ipc.ProgInfo, calls prog.Contender, opts knotterOpts) []interleaving.Segment {
	knotter := scheduler.GetKnotter(
		opts.collected,
		&proc.fuzzer.signalMu,
	)
	if opts.reassignThreadID {
		knotter.SetReassignThreadID()
	}
	if opts.strictTimestamp {
		knotter.SetStrictTimestamp()
	}

	seq := sequentialAccesses(info, calls)
	if !knotter.AddSequentialTrace(seq) {
		return nil
	}
	knotter.ExcavateKnots()

	return knotter.GetKnots()
}

func sequentialAccesses(info *ipc.ProgInfo, calls prog.Contender) (seq []interleaving.SerialAccess) {
	for _, call := range calls.Calls {
		seq = append(seq, info.Calls[call].Access)
	}
	return
}

func (proc *Proc) enqueueCallTriage(p *prog.Prog, flags ProgTypes, callIndex int, info ipc.CallInfo) {
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

func (proc *Proc) enqueueThreading(p *prog.Prog, calls prog.Contender, knots []interleaving.Segment) {
	proc.fuzzer.addCollection(CollectionThreadingHint, uint64(len(knots)))
	proc.fuzzer.workQueue.enqueue(&WorkThreading{
		p:     p.Clone(),
		calls: calls,
		knots: knots,
	})
}

func (proc *Proc) executeRaw(opts *ipc.ExecOpts, p *prog.Prog, stat Stat) *ipc.ProgInfo {
	proc.executed++
	if opts.Flags&ipc.FlagDedupCover == 0 {
		log.Fatalf("dedup cover is not enabled")
	}
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
				// but so far we don't have a better handling than ignoring this.
				// This error is observed a lot on the seeded syz_mount_image calls.
				return nil
			}
			if try > 10 {
				log.Fatalf("executor %v failed %v times:\n%v", proc.pid, try, err)
			}
			log.Logf(4, "fuzzer detected executor failure='%v', retrying #%d", err, try+1)
			debug.FreeOSMemory()
			time.Sleep(time.Second)
			continue
		}

		proc.shiftAccesses(info)

		retry := needRetry(p, info)
		proc.logResult(p, info, hanged, retry)
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

func (proc *Proc) logProgram(opts *ipc.ExecOpts, p *prog.Prog) {
	if proc.fuzzer.outputType == OutputNone {
		return
	}

	data := p.Serialize()
	strOpts := ""
	if opts.Flags&ipc.FlagInjectFault != 0 {
		strOpts = fmt.Sprintf(" (fault-call:%v fault-nth:%v)", opts.FaultCall, opts.FaultNth)
	}
	if p.Threaded {
		strOpts += fmt.Sprintf(" (threaded %v) ", p.Contender.Calls)
	}

	// The following output helps to understand what program crashed kernel.
	// It must not be intermixed.
	switch proc.fuzzer.outputType {
	case OutputStdout:
		now := time.Now()
		proc.fuzzer.logMu.Lock()
		fmt.Printf("%02v:%02v:%02v executing program (%d calls) %v%v:\n%s\n",
			now.Hour(), now.Minute(), now.Second(), len(p.Calls),
			proc.pid, strOpts, data)
		proc.fuzzer.logMu.Unlock()
	case OutputDmesg:
		fd, err := syscall.Open("/dev/kmsg", syscall.O_WRONLY, 0)
		if err == nil {
			buf := new(bytes.Buffer)
			fmt.Fprintf(buf, "syzkaller: executing program %v%v:\n%s\n",
				proc.pid, strOpts, data)
			syscall.Write(fd, buf.Bytes())
			syscall.Close(fd)
		}
	case OutputFile:
		f, err := os.Create(fmt.Sprintf("%v-%v.prog", proc.fuzzer.name, proc.pid))
		if err == nil {
			if strOpts != "" {
				fmt.Fprintf(f, "#%v\n", strOpts)
			}
			f.Write(data)
			f.Close()
		}
	default:
		log.Fatalf("unknown output type: %v", proc.fuzzer.outputType)
	}
}

type ResultLogger struct {
	p          *prog.Prog
	info       *ipc.ProgInfo
	threads    uint64
	epochs     uint64
	outputType OutputType
	column     int
}

func (proc *Proc) logResult(p *prog.Prog, info *ipc.ProgInfo, hanged, retry bool) {
	if proc.fuzzer.outputType == OutputNone {
		return
	}

	threads, epochs := p.Frame()
	logger := ResultLogger{
		p:          p,
		info:       info,
		threads:    threads,
		epochs:     epochs,
		outputType: proc.fuzzer.outputType,
	}
	(&logger).initialize()

	proc.fuzzer.logMu.Lock()
	defer proc.fuzzer.logMu.Unlock()

	logger.logHeader()
	for i := uint64(0); i < epochs; i++ {
		logger.logEpochLocked(i)
	}
	log.Logf(2, "Retry: %v", retry)
	logger.logFootprint()
}

func (logger *ResultLogger) initialize() {
	logger.column = len("thread#0")
	for _, c := range logger.p.Calls {
		l := len(c.Meta.Name)
		if l > logger.column {
			logger.column = l
		}
	}
	logger.column += 2
}

func (logger ResultLogger) logHeader() {
	header := []string{}
	for i := uint64(0); i < logger.threads; i++ {
		header = append(header, fmt.Sprintf("thread%d", i))
	}
	logger.logRowLocked(header)
}

func (logger ResultLogger) logEpochLocked(epoch uint64) {
	m := make(map[uint64]string)
	for _, c := range logger.p.Calls {
		if c.Epoch == epoch {
			m[c.Thread] = c.Meta.Name
		}
	}
	row := []string{}
	for i := uint64(0); i < logger.threads; i++ {
		str := "(empty)"
		if str0, ok := m[i]; ok {
			str = str0
		}
		row = append(row, str)
	}
	logger.logRowLocked(row)
}

func (logger ResultLogger) logRowLocked(row []string) {
	switch logger.outputType {
	case OutputStdout:
		s := ""
		for _, r := range row {
			s += r
			s += strings.Repeat(" ", logger.column-len(r))
		}
		log.Logf(2, "%s", s)
	default:
		// XXX: We support standard output only, but don't want to
		// quit with others
	}
}

func (logger ResultLogger) logFootprint() {
	log.Logf(2, "Footprint")
	for i, inf := range logger.info.Calls {
		if len(inf.SchedpointOutcome) == 0 {
			continue
		}
		str := fmt.Sprintf("Call #%d: ", i)
		for _, outcome := range inf.SchedpointOutcome {
			str += fmt.Sprintf("(%d, %d) ", outcome.Order, outcome.Footprint)
		}
		log.Logf(2, "%s", str)
	}
}
