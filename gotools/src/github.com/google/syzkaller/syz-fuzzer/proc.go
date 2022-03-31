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
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/primitive"
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
	// To give a half of computing power for scheduling
	executed  uint64
	scheduled uint64
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
	proc := &Proc{
		fuzzer:            fuzzer,
		pid:               pid,
		env:               env,
		rnd:               rnd,
		execOpts:          fuzzer.execOpts,
		execOptsCover:     &execOptsCover,
		execOptsComps:     &execOptsComps,
		execOptsNoCollide: &execOptsNoCollide,
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
		MonitorMemUsage()
		if proc.needScheduling() {
			fuzzerSnapshot := proc.fuzzer.snapshot()
			proc.scheduleInput(fuzzerSnapshot, false)
			continue
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
			proc.scheduleInput(fuzzerSnapshot, true)
		}
		if i%100 == 0 && !affinity.RunOnCPU(1<<0) {
			log.Logf(0, "[WARN] Proc goroutine #%v runs on CPU other than 0", proc.pid)
		}
	}
}

func (proc *Proc) needScheduling() bool {
	thold := 5
	if !proc.fuzzer.generate {
		thold = 1
	}
	if len(proc.fuzzer.threadedCorpus) < thold {
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

func (proc *Proc) scheduleInput(fuzzerSnapshot FuzzerSnapshot, force bool) {
	// proc.scheduleInput() does not queue additional works, so
	// executing proc.scheduleInput() does not cause the workqueues
	// exploding.
	for cnt := 0; cnt < 10 && (proc.needScheduling() || force); cnt++ {
		force = false
		// increase scheduled here so let needScheduling() know that
		// we tried to mutate a schedule.
		proc.scheduled++
		tp := fuzzerSnapshot.chooseThreadedProgram(proc.rnd)
		if tp == nil {
			continue
		}
		p := tp.P.Clone()

		log.Logf(1, "proc #%v: scheduling an input", proc.pid)
		ok, remaining := p.MutateScheduleFromHint(proc.rnd, tp.Hint)
		// We exclude used knots from tp.Hint even if
		// p.MutateScheduleFromHint() fails because it will fail later
		// anyway.
		tp.Hint = remaining
		if !ok {
			log.Logf(0, "[WARN] failed to mutate a schedule")
			continue
		}
		proc.execute(proc.execOpts, p, ProgNormal, StatSchedule)
	}
}

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
	p := item.p.Clone()
	p.Threading(item.calls)
	knotter := scheduler.Knotter{}
	for i := 0; i < 2; i++ {
		inf := proc.executeRaw(proc.execOpts, p, StatThreading)
		seq := sequentialTrace(inf, p.Threaded)
		if !knotter.AddSequentialTrace(seq) {
			log.Logf(0, "[WARN] failed to add the sequence trace")
			return
		}
		p.Reverse()
	}
	knotter.ExcavateKnots()
	knots := knotter.GetKnots()

	newKnots := proc.fuzzer.newKnot(knots)
	newKnots = append(newKnots, primitive.Intersect(knots, item.knots)...)

	// Now we know newly found knots
	proc.fuzzer.addThreadedInputToCorpus(p, newKnots)
}

func sequentialTrace(info *ipc.ProgInfo, threaded bool) []primitive.SerialAccess {
	if !threaded {
		return nil
	}
	res := []primitive.SerialAccess{}
	for _, c := range info.Calls {
		if len(c.Access) != 0 {
			res = append(res, c.Access)
		}
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
	maxIntermediateCalls := 3
	for c1 := 0; c1 < len(p.Calls); c1++ {
		for c2 := c1 + 1; c2 < len(p.Calls) && c2-c1-1 < maxIntermediateCalls; c2++ {
			cont := prog.Contender{Calls: []int{c1, c2}}

			knotter := scheduler.Knotter{ReassignThreadID: true}
			if !knotter.AddSequentialTrace([]primitive.SerialAccess{info.Calls[c1].Access, info.Calls[c2].Access}) {
				log.Logf(0, "[WARN] failed to add the sequence trace")
				continue
			}
			knotter.ExcavateKnots()
			comms := knotter.GetCommunications()
			knots := knotter.GetKnots()

			if newComms, newKnots := proc.fuzzer.newCommunication(comms), proc.fuzzer.newKnot(knots); len(newComms) != 0 || len(newKnots) != 0 {
				proc.enqueueThreading(p, cont, newKnots)
			}
		}
	}
}

func (proc *Proc) postExecuteThreaded(p *prog.Prog, info *ipc.ProgInfo) *ipc.ProgInfo {
	// TODO:
	return info
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

func (proc *Proc) enqueueThreading(p *prog.Prog, calls prog.Contender, knots []primitive.Segment) {
	if proc.fuzzer.shutOffThreading(p, calls) {
		return
	}
	// proc.fuzzer.mergeMaxReadFrom(p, calls, info)
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
		retry := false
		for _, ci := range p.Contender.Calls {
			inf := info.Calls[ci]
			if inf.Flags&ipc.CallRetry != 0 {
				retry = true
				break
			}
		}
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

func buildScheduleFilter(p *prog.Prog, info *ipc.ProgInfo) []uint32 {
	const FOOTPRINT_MISSED = 1
	cnt := 0
	filter := make([]uint32, p.Schedule.Len())
	for _, ci := range info.Calls {
		cnt += len(ci.SchedpointOutcome)
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
	if cnt != p.Schedule.Len() {
		return nil
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
	logger.logNewCoverage()
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
		// TODO: We support standard output only, but don't want to
		// quit with others
	}
}

func (logger ResultLogger) logNewCoverage() {
	// TODO
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
