// Copyright 2015 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	golog "log"
	"math/rand"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/syzkaller/pkg/affinity"
	"github.com/google/syzkaller/pkg/csource"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/host"
	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/ipc/ipcconfig"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/osutil"
	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/pkg/signal"
	"github.com/google/syzkaller/pkg/tool"
	"github.com/google/syzkaller/prog"
	_ "github.com/google/syzkaller/sys"
	"github.com/google/syzkaller/sys/targets"
)

type Fuzzer struct {
	name              string
	outputType        OutputType
	config            *ipc.Config
	execOpts          *ipc.ExecOpts
	procs             []*Proc
	gate              *ipc.Gate
	workQueue         *WorkQueue
	needPoll          chan struct{}
	choiceTable       *prog.ChoiceTable
	stats             [StatCount]uint64
	manager           *rpctype.RPCClient
	target            *prog.Target
	triagedCandidates uint32
	timeouts          targets.Timeouts

	faultInjectionEnabled    bool
	comparisonTracingEnabled bool

	corpusMu       sync.RWMutex
	corpus         []*prog.Prog
	corpusHashes   map[hash.Sig]struct{}
	corpusPrios    []int64
	sumPrios       int64
	threadedCorpus []*prog.ThreadedProg
	staleCount     map[uint32]int

	signalMu       sync.RWMutex
	corpusSignal   signal.Signal // signal of inputs in corpus
	maxSignal      signal.Signal // max signal ever observed including flakes
	newSignal      signal.Signal // diff of maxSignal since last sync with master
	corpusReadFrom signal.ReadFrom
	maxReadFrom    signal.ReadFrom
	newReadFrom    signal.ReadFrom

	// Mostly for debugging scheduling mutation. If generate is true,
	// procs do not generate/mutate inputs but schedule
	generate bool

	checkResult *rpctype.CheckArgs
	logMu       sync.Mutex
}

type FuzzerSnapshot struct {
	corpus         []*prog.Prog
	threadedCorpus []*prog.ThreadedProg
	corpusPrios    []int64
	sumPrios       int64
}

type Stat int

const (
	StatGenerate Stat = iota
	StatFuzz
	StatCandidate
	StatTriage
	StatMinimize
	StatSmash
	StatHint
	StatSeed
	StatThreading
	StatSchedule
	StatCount
)

var statNames = [StatCount]string{
	StatGenerate:  "exec gen",
	StatFuzz:      "exec fuzz",
	StatCandidate: "exec candidate",
	StatTriage:    "exec triage",
	StatMinimize:  "exec minimize",
	StatSmash:     "exec smash",
	StatHint:      "exec hints",
	StatSeed:      "exec seeds",
	StatThreading: "exec threadings",
	StatSchedule:  "exec schedulings",
}

type OutputType int

const (
	OutputNone OutputType = iota
	OutputStdout
	OutputDmesg
	OutputFile
)

func createIPCConfig(features *host.Features, config *ipc.Config) {
	if features[host.FeatureExtraCoverage].Enabled {
		config.Flags |= ipc.FlagExtraCover
	}
	if features[host.FeatureNetInjection].Enabled {
		config.Flags |= ipc.FlagEnableTun
	}
	if features[host.FeatureNetDevices].Enabled {
		config.Flags |= ipc.FlagEnableNetDev
	}
	config.Flags |= ipc.FlagEnableNetReset
	config.Flags |= ipc.FlagEnableCgroups
	config.Flags |= ipc.FlagEnableCloseFds
	if features[host.FeatureDevlinkPCI].Enabled {
		config.Flags |= ipc.FlagEnableDevlinkPCI
	}
	if features[host.FeatureVhciInjection].Enabled {
		config.Flags |= ipc.FlagEnableVhciInjection
	}
	if features[host.FeatureWifiEmulation].Enabled {
		config.Flags |= ipc.FlagEnableWifi
	}
}

// nolint: funlen
func main() {
	golog.SetPrefix("[FUZZER] ")
	debug.SetGCPercent(50)
	resetKSSB()

	var (
		flagName    = flag.String("name", "test", "unique name for manager")
		flagOS      = flag.String("os", runtime.GOOS, "target OS")
		flagArch    = flag.String("arch", runtime.GOARCH, "target arch")
		flagManager = flag.String("manager", "", "manager rpc address")
		flagProcs   = flag.Int("procs", 1, "number of parallel test processes")
		flagOutput  = flag.String("output", "stdout", "write programs to none/stdout/dmesg/file")
		flagTest    = flag.Bool("test", false, "enable image testing mode")      // used by syz-ci
		flagRunTest = flag.Bool("runtest", false, "enable program testing mode") // used by pkg/runtest
		flagGen     = flag.Bool("gen", true, "generate/mutate inputs")
	)
	defer tool.Init()()
	outputType := parseOutputType(*flagOutput)
	log.Logf(0, "fuzzer started")

	target, err := prog.GetTarget(*flagOS, *flagArch)
	if err != nil {
		log.Fatalf("%v", err)
	}

	config, execOpts, err := ipcconfig.Default(target)
	if err != nil {
		log.Fatalf("failed to create default ipc config: %v", err)
	}
	timeouts := config.Timeouts
	sandbox := ipc.FlagsToSandbox(config.Flags)
	shutdown := make(chan struct{})
	osutil.HandleInterrupts(shutdown)
	go func() {
		// Handles graceful preemption on GCE.
		<-shutdown
		log.Logf(0, "SYZ-FUZZER: PREEMPTED")
		os.Exit(1)
	}()

	checkArgs := &checkArgs{
		target:         target,
		sandbox:        sandbox,
		ipcConfig:      config,
		ipcExecOpts:    execOpts,
		gitRevision:    prog.GitRevision,
		targetRevision: target.Revision,
	}
	if *flagTest {
		testImage(*flagManager, checkArgs)
		return
	}

	machineInfo, modules := collectMachineInfos(target)

	log.Logf(0, "dialing manager at %v", *flagManager)
	manager, err := rpctype.NewRPCClient(*flagManager, timeouts.Scale)
	if err != nil {
		log.Fatalf("failed to connect to manager: %v ", err)
	}

	log.Logf(1, "connecting to manager...")
	a := &rpctype.ConnectArgs{
		Name:        *flagName,
		MachineInfo: machineInfo,
		Modules:     modules,
	}
	r := &rpctype.ConnectRes{}
	if err := manager.Call("Manager.Connect", a, r); err != nil {
		log.Fatalf("failed to connect to manager: %v ", err)
	}
	featureFlags, err := csource.ParseFeaturesFlags("none", "none", true)
	if err != nil {
		log.Fatal(err)
	}
	if r.CoverFilterBitmap != nil {
		if err := osutil.WriteFile("syz-cover-bitmap", r.CoverFilterBitmap); err != nil {
			log.Fatalf("failed to write syz-cover-bitmap: %v", err)
		}
	}
	if r.CheckResult == nil {
		checkArgs.gitRevision = r.GitRevision
		checkArgs.targetRevision = r.TargetRevision
		checkArgs.enabledCalls = r.EnabledCalls
		checkArgs.allSandboxes = r.AllSandboxes
		checkArgs.featureFlags = featureFlags
		r.CheckResult, err = checkMachine(checkArgs)
		if err != nil {
			if r.CheckResult == nil {
				r.CheckResult = new(rpctype.CheckArgs)
			}
			r.CheckResult.Error = err.Error()
		}
		r.CheckResult.Name = *flagName
		if err := manager.Call("Manager.Check", r.CheckResult, nil); err != nil {
			log.Fatalf("Manager.Check call failed: %v", err)
		}
		if r.CheckResult.Error != "" {
			log.Fatalf("%v", r.CheckResult.Error)
		}
	} else {
		target.UpdateGlobs(r.CheckResult.GlobFiles)
		if err = host.Setup(target, r.CheckResult.Features, featureFlags, config.Executor); err != nil {
			log.Fatal(err)
		}
	}
	log.Logf(0, "syscalls: %v", len(r.CheckResult.EnabledCalls[sandbox]))
	for _, feat := range r.CheckResult.Features.Supported() {
		log.Logf(0, "%v: %v", feat.Name, feat.Reason)
	}
	createIPCConfig(r.CheckResult.Features, config)

	if *flagRunTest {
		runTest(target, manager, *flagName, config.Executor)
		return
	}

	needPoll := make(chan struct{}, 1)
	needPoll <- struct{}{}
	fuzzer := &Fuzzer{
		name:                     *flagName,
		outputType:               outputType,
		config:                   config,
		execOpts:                 execOpts,
		workQueue:                newWorkQueue(*flagProcs, needPoll),
		needPoll:                 needPoll,
		manager:                  manager,
		target:                   target,
		timeouts:                 timeouts,
		faultInjectionEnabled:    false,
		comparisonTracingEnabled: false,
		corpusHashes:             make(map[hash.Sig]struct{}),
		corpusReadFrom:           signal.NewReadFrom(),
		maxReadFrom:              signal.NewReadFrom(),
		newReadFrom:              signal.NewReadFrom(),
		staleCount:               make(map[uint32]int),
		checkResult:              r.CheckResult,
		generate:                 *flagGen,
	}
	gateCallback := fuzzer.useBugFrames(r, *flagProcs)
	fuzzer.gate = ipc.NewGate(2**flagProcs, gateCallback)

	for needCandidates, more := true, true; more; needCandidates = false {
		more = fuzzer.poll(needCandidates, nil)
		// This loop lead to "no output" in qemu emulation, tell manager we are not dead.
		log.Logf(0, "fetching corpus: %v, signal %v/%v (executing program)",
			len(fuzzer.corpus), len(fuzzer.corpusSignal), len(fuzzer.maxSignal))
	}
	calls := make(map[*prog.Syscall]bool)
	for _, id := range r.CheckResult.EnabledCalls[sandbox] {
		calls[target.Syscalls[id]] = true
	}
	fuzzer.choiceTable = target.BuildChoiceTable(fuzzer.corpus, calls)

	if r.CoverFilterBitmap != nil {
		fuzzer.execOpts.Flags |= ipc.FlagEnableCoverageFilter
	}

	log.Logf(0, "starting %v fuzzer processes", *flagProcs)
	if !fuzzer.generate {
		log.Logf(0, "fuzzer will not generate/mutate inputs")
	}
	for pid := 0; pid < *flagProcs; pid++ {
		proc, err := newProc(fuzzer, pid)
		if err != nil {
			log.Fatalf("failed to create proc: %v", err)
		}
		fuzzer.procs = append(fuzzer.procs, proc)
		go proc.loop()
	}

	fuzzer.pollLoop()
}

func collectMachineInfos(target *prog.Target) ([]byte, []host.KernelModule) {
	machineInfo, err := host.CollectMachineInfo()
	if err != nil {
		log.Fatalf("failed to collect machine information: %v", err)
	}
	modules, err := host.CollectModulesInfo()
	if err != nil {
		log.Fatalf("failed to collect modules info: %v", err)
	}
	return machineInfo, modules
}

// Returns gateCallback for leak checking if enabled.
func (fuzzer *Fuzzer) useBugFrames(r *rpctype.ConnectRes, flagProcs int) func() {
	var gateCallback func()

	if r.CheckResult.Features[host.FeatureLeak].Enabled {
		gateCallback = func() { fuzzer.gateCallback(r.MemoryLeakFrames) }
	}

	if r.CheckResult.Features[host.FeatureKCSAN].Enabled && len(r.DataRaceFrames) != 0 {
		fuzzer.filterDataRaceFrames(r.DataRaceFrames)
	}

	return gateCallback
}

func (fuzzer *Fuzzer) gateCallback(leakFrames []string) {
	// Leak checking is very slow so we don't do it while triaging the corpus
	// (otherwise it takes infinity). When we have presumably triaged the corpus
	// (triagedCandidates == 1), we run leak checking bug ignore the result
	// to flush any previous leaks. After that (triagedCandidates == 2)
	// we do actual leak checking and report leaks.
	triagedCandidates := atomic.LoadUint32(&fuzzer.triagedCandidates)
	if triagedCandidates == 0 {
		return
	}
	args := append([]string{"leak"}, leakFrames...)
	timeout := fuzzer.timeouts.NoOutput * 9 / 10
	output, err := osutil.RunCmd(timeout, "", fuzzer.config.Executor, args...)
	if err != nil && triagedCandidates == 2 {
		// If we exit right away, dying executors will dump lots of garbage to console.
		os.Stdout.Write(output)
		fmt.Printf("BUG: leak checking failed\n")
		time.Sleep(time.Hour)
		os.Exit(1)
	}
	if triagedCandidates == 1 {
		atomic.StoreUint32(&fuzzer.triagedCandidates, 2)
	}
}

func (fuzzer *Fuzzer) filterDataRaceFrames(frames []string) {
	args := append([]string{"setup_kcsan_filterlist"}, frames...)
	timeout := time.Minute * fuzzer.timeouts.Scale
	output, err := osutil.RunCmd(timeout, "", fuzzer.config.Executor, args...)
	if err != nil {
		log.Fatalf("failed to set KCSAN filterlist: %v", err)
	}
	log.Logf(0, "%s", output)
}

func (fuzzer *Fuzzer) pollLoop() {
	var execTotal uint64
	var lastPoll time.Time
	var lastPrint time.Time
	ticker := time.NewTicker(3 * time.Second * fuzzer.timeouts.Scale).C
	for {
		poll := false
		select {
		case <-ticker:
		case <-fuzzer.needPoll:
			poll = true
		}
		if fuzzer.outputType != OutputStdout && time.Since(lastPrint) > 10*time.Second*fuzzer.timeouts.Scale {
			// Keep-alive for manager.
			log.Logf(0, "alive, executed %v", execTotal)
			lastPrint = time.Now()
		}
		if poll || time.Since(lastPoll) > 10*time.Second*fuzzer.timeouts.Scale {
			needCandidates := fuzzer.workQueue.wantCandidates()
			if poll && !needCandidates {
				continue
			}
			stats := make(map[string]uint64)
			for _, proc := range fuzzer.procs {
				stats["exec total"] += atomic.SwapUint64(&proc.env.StatExecs, 0)
				stats["executor restarts"] += atomic.SwapUint64(&proc.env.StatRestarts, 0)
			}
			for stat := Stat(0); stat < StatCount; stat++ {
				v := atomic.SwapUint64(&fuzzer.stats[stat], 0)
				stats[statNames[stat]] = v
				execTotal += v
			}
			if !fuzzer.poll(needCandidates, stats) {
				lastPoll = time.Now()
			}
			if !affinity.RunOnCPU(1 << 0) {
				log.Logf(0, "[WARN] Fuzzer goroutine runs on CPU other than 0")
			}
		}
	}
}

func (fuzzer *Fuzzer) poll(needCandidates bool, stats map[string]uint64) bool {
	a := &rpctype.PollArgs{
		Name:           fuzzer.name,
		NeedCandidates: needCandidates,
		MaxSignal:      fuzzer.grabNewSignal().Serialize(),
		MaxReadFrom:    fuzzer.grabNewReadFrom().Serialize(),
		Stats:          stats,
	}
	r := &rpctype.PollRes{}
	if err := fuzzer.manager.Call("Manager.Poll", a, r); err != nil {
		log.Fatalf("Manager.Poll call failed: %v", err)
	}
	maxSignal := r.MaxSignal.Deserialize()
	maxReadFrom := r.MaxReadFrom.Deserialize()
	log.Logf(1, "poll: candidates=%v inputs=%v signal=%v readfrom=%v",
		len(r.Candidates), len(r.NewInputs), maxSignal.Len(), maxReadFrom.Len())
	fuzzer.addMaxSignal(maxSignal)
	fuzzer.addMaxReadFrom(maxReadFrom)
	for _, inp := range r.NewInputs {
		fuzzer.addInputFromAnotherFuzzer(inp)
	}
	// for _, inp := range r.NewThreadedInputs {
	// 	fuzzer.addThreadedInputFromAnotherFuzzer(inp)
	// }
	for _, candidate := range r.Candidates {
		fuzzer.addCandidateInput(candidate)
	}
	if needCandidates && len(r.Candidates) == 0 && atomic.LoadUint32(&fuzzer.triagedCandidates) == 0 {
		atomic.StoreUint32(&fuzzer.triagedCandidates, 1)
	}
	return len(r.NewInputs) != 0 || len(r.Candidates) != 0 || maxSignal.Len() != 0
}

func (fuzzer *Fuzzer) sendInputToManager(inp rpctype.RPCInput) {
	a := &rpctype.NewInputArgs{
		Name:     fuzzer.name,
		RPCInput: inp,
	}
	if err := fuzzer.manager.Call("Manager.NewInput", a, nil); err != nil {
		log.Fatalf("Manager.NewInput call failed: %v", err)
	}
}

// func (fuzzer *Fuzzer) sendThreadedInputToManager(inp rpctype.RPCThreadedInput) {
// 	a := &rpctype.NewThreadedInputArgs{
// 		Name:             fuzzer.name,
// 		RPCThreadedInput: inp,
// 	}
// 	if err := fuzzer.manager.Call("Manager.NewThreadedInput", a, nil); err != nil {
// 		log.Fatalf("Manager.NewThreadedInput call failed: %v", err)
// 	}
// }

func (fuzzer *Fuzzer) addInputFromAnotherFuzzer(inp rpctype.RPCInput) {
	p := fuzzer.deserializeInput(inp.Prog)
	if p == nil {
		return
	}
	sig := hash.Hash(inp.Prog)
	sign := inp.Signal.Deserialize()
	fuzzer.addInputToCorpus(p, sign, sig)
}

// func (fuzzer *Fuzzer) addThreadedInputFromAnotherFuzzer(inp rpctype.RPCThreadedInput) {
// 	p := fuzzer.deserializeInput(inp.Prog)
// 	if p == nil {
// 		return
// 	}
// 	readfrom := inp.ReadFrom.Deserialize()
// 	fuzzer.addInputToThreadedCorpus(p, readfrom, inp.Serial)
// }

func (fuzzer *Fuzzer) addCandidateInput(candidate rpctype.RPCCandidate) {
	p := fuzzer.deserializeInput(candidate.Prog)
	if p == nil {
		return
	}
	flags := ProgCandidate
	if candidate.Minimized {
		flags |= ProgMinimized
	}
	if candidate.Smashed {
		flags |= ProgSmashed
	}
	fuzzer.workQueue.enqueue(&WorkCandidate{
		p:     p,
		flags: flags,
	})
}

func (fuzzer *Fuzzer) deserializeInput(inp []byte) *prog.Prog {
	p, err := fuzzer.target.Deserialize(inp, prog.NonStrict)
	if err != nil {
		log.Fatalf("failed to deserialize prog: %v\n%s", err, inp)
	}
	// We build choice table only after we received the initial corpus,
	// so we don't check the initial corpus here, we check it later in BuildChoiceTable.
	if fuzzer.choiceTable != nil {
		fuzzer.checkDisabledCalls(p)
	}
	if len(p.Calls) > prog.MaxCalls {
		return nil
	}
	return p
}

func (fuzzer *Fuzzer) checkDisabledCalls(p *prog.Prog) {
	for _, call := range p.Calls {
		if !fuzzer.choiceTable.Enabled(call.Meta.ID) {
			fmt.Printf("executing disabled syscall %v [%v]\n", call.Meta.Name, call.Meta.ID)
			sandbox := ipc.FlagsToSandbox(fuzzer.config.Flags)
			fmt.Printf("check result for sandbox=%v:\n", sandbox)
			for _, id := range fuzzer.checkResult.EnabledCalls[sandbox] {
				meta := fuzzer.target.Syscalls[id]
				fmt.Printf("  %v [%v]\n", meta.Name, meta.ID)
			}
			fmt.Printf("choice table:\n")
			for i, meta := range fuzzer.target.Syscalls {
				fmt.Printf("  #%v: %v [%v]: enabled=%v\n", i, meta.Name, meta.ID, fuzzer.choiceTable.Enabled(meta.ID))
			}
			panic("disabled syscall")
		}
	}
}

func (fuzzer *FuzzerSnapshot) chooseProgram(r *rand.Rand) *prog.Prog {
	randVal := r.Int63n(fuzzer.sumPrios + 1)
	idx := sort.Search(len(fuzzer.corpusPrios), func(i int) bool {
		return fuzzer.corpusPrios[i] >= randVal
	})
	return fuzzer.corpus[idx]
}

// func (fuzzer *FuzzerSnapshot) chooseThreadedProgram(r *rand.Rand) *prog.ThreadedProg {
// 	if len(fuzzer.threadedCorpus) == 0 {
// 		return nil
// 	}
// 	// NOTE: we want to select a threaded program with the long legnth
// 	// of read-from (i.e., the scheduling space is large), and has not
// 	// been selected too many times (i.e., and we don't expore the
// 	// scheduling space yet).
// 	// XXX: Although the idea is straight-forward, implementing it is
// 	// costly (or not. whatever.). So instead, the below is a kind of
// 	// heuristic (hopefully) mimicking the idea.
// 	for try := 0; try < 10; try++ {
// 		idx := r.Intn(len(fuzzer.threadedCorpus))
// 		tp := fuzzer.threadedCorpus[idx]
// 		if tp.Prio-tp.Scheduled >= r.Intn(tp.Prio) {
// 			tp.Scheduled++
// 			return tp
// 		}
// 	}
// 	idx := r.Intn(len(fuzzer.threadedCorpus))
// 	return fuzzer.threadedCorpus[idx]
// }

func (fuzzer *Fuzzer) __addInputToCorpus(p *prog.Prog, sig hash.Sig, prio int64) {
	fuzzer.corpusMu.Lock()
	defer fuzzer.corpusMu.Unlock()
	if _, ok := fuzzer.corpusHashes[sig]; !ok {
		fuzzer.corpus = append(fuzzer.corpus, p)
		fuzzer.corpusHashes[sig] = struct{}{}
		fuzzer.sumPrios += prio
		fuzzer.corpusPrios = append(fuzzer.corpusPrios, fuzzer.sumPrios)
	}
}

func (fuzzer *Fuzzer) addInputToCorpus(p *prog.Prog, sign signal.Signal, sig hash.Sig) {
	prio := int64(len(sign))
	if sign.Empty() {
		prio = 1
	}
	fuzzer.__addInputToCorpus(p, sig, prio)

	if !sign.Empty() {
		fuzzer.signalMu.Lock()
		fuzzer.corpusSignal.Merge(sign)
		fuzzer.maxSignal.Merge(sign)
		fuzzer.signalMu.Unlock()
	}
}

// func (fuzzer *Fuzzer) addInputToThreadedCorpus(p *prog.Prog, readfrom signal.ReadFrom, serial primitive.SerialAccess) {
// 	fuzzer.corpusMu.Lock()
// 	defer fuzzer.corpusMu.Unlock()
// 	fuzzer.threadedCorpus = append(fuzzer.threadedCorpus, &prog.ThreadedProg{
// 		P:        p,
// 		ReadFrom: readfrom,
// 		Serial:   serial,
// 		Prio:     readfrom.Len() * 2,
// 	})
// }

// func (fuzzer *Fuzzer) addThreadedInputToCorpus(p *prog.Prog, info *ipc.ProgInfo, sig hash.Sig) {
// 	// TODO: how to set the priority of threaded input?
// 	rf := info.ContenderReadFrom(p.Contender)
// 	serial := info.ContenderSerialAccess(p.Contender)

// 	const threadedPrio = 1000
// 	fuzzer.__addInputToCorpus(p, sig, threadedPrio)
// 	fuzzer.addInputToThreadedCorpus(p, rf, serial)

// 	fuzzer.signalMu.Lock()
// 	defer fuzzer.signalMu.Unlock()

// 	fuzzer.corpusReadFrom.Merge(rf)
// 	fuzzer.maxReadFrom.Merge(rf)
// }

func (fuzzer *Fuzzer) snapshot() FuzzerSnapshot {
	fuzzer.corpusMu.RLock()
	defer fuzzer.corpusMu.RUnlock()
	return FuzzerSnapshot{fuzzer.corpus, fuzzer.threadedCorpus, fuzzer.corpusPrios, fuzzer.sumPrios}
}

func (fuzzer *Fuzzer) addMaxSignal(sign signal.Signal) {
	if sign.Len() == 0 {
		return
	}
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	fuzzer.maxSignal.Merge(sign)
}

func (fuzzer *Fuzzer) grabNewSignal() signal.Signal {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	sign := fuzzer.newSignal
	if sign.Empty() {
		return nil
	}
	fuzzer.newSignal = nil
	return sign
}

func (fuzzer *Fuzzer) grabNewReadFrom() signal.ReadFrom {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	rf := fuzzer.newReadFrom
	if rf.Empty() {
		return nil
	}
	fuzzer.newReadFrom = signal.NewReadFrom()
	return rf
}

func (fuzzer *Fuzzer) corpusSignalDiff(sign signal.Signal) signal.Signal {
	fuzzer.signalMu.RLock()
	defer fuzzer.signalMu.RUnlock()
	return fuzzer.corpusSignal.Diff(sign)
}

func (fuzzer *Fuzzer) checkNewSignal(p *prog.Prog, info *ipc.ProgInfo) (calls []int, extra bool) {
	fuzzer.signalMu.RLock()
	defer fuzzer.signalMu.RUnlock()
	for i, inf := range info.Calls {
		if fuzzer.checkNewCallSignal(p, &inf, i) {
			calls = append(calls, i)
		}
	}
	extra = fuzzer.checkNewCallSignal(p, &info.Extra, -1)
	return
}

func (fuzzer *Fuzzer) checkNewCallSignal(p *prog.Prog, info *ipc.CallInfo, call int) bool {
	diff := fuzzer.maxSignal.DiffRaw(info.Signal, signalPrio(p, info, call))
	if diff.Empty() {
		return false
	}
	fuzzer.signalMu.RUnlock()
	fuzzer.signalMu.Lock()
	fuzzer.maxSignal.Merge(diff)
	fuzzer.newSignal.Merge(diff)
	fuzzer.signalMu.Unlock()
	fuzzer.signalMu.RLock()
	return true
}

func (fuzzer *Fuzzer) mergeMaxReadFrom(p *prog.Prog, contender prog.Contender, info *ipc.ProgInfo) {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	rf := info.ContenderReadFrom(contender)
	fuzzer.maxReadFrom.Merge(rf)
	fuzzer.newReadFrom.Merge(rf)
}

func (fuzzer *Fuzzer) addMaxReadFrom(rf signal.ReadFrom) {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	fuzzer.maxReadFrom.Merge(rf)
}

func (fuzzer *Fuzzer) __checkNewReadFrom(p *prog.Prog, contender prog.Contender, info *ipc.ProgInfo, readfrom signal.ReadFrom) bool {
	rf := info.ContenderReadFrom(contender)
	fuzzer.signalMu.RLock()
	defer fuzzer.signalMu.RUnlock()
	diff := readfrom.Diff(rf)
	return !diff.Empty()
}

func (fuzzer *Fuzzer) checkNewReadFrom(p *prog.Prog, contender prog.Contender, info *ipc.ProgInfo) bool {
	return fuzzer.__checkNewReadFrom(p, contender, info, fuzzer.corpusReadFrom)
}

func (fuzzer *Fuzzer) checkMaxReadFrom(p *prog.Prog, contender prog.Contender, info *ipc.ProgInfo) bool {
	// if diff.Empty(), obtained read-from (i.e., depends
	// relationship) does not give a clue for interesting read-from
	// (i.e., conflicts). It might provide interesting results if we
	// execute the threaded p, but it is somewhat unlikely. Give this
	// case very low chance (i.e., 1%) to go into the threading
	// workqueue.
	return fuzzer.__checkNewReadFrom(p, contender, info, fuzzer.maxReadFrom) || rand.Intn(100) == 0
}

func (fuzzer *Fuzzer) identifyContenders(p *prog.Prog, info *ipc.ProgInfo) (res []prog.Contender) {
	// identify calls that are likely to be of interest when run
	// in parallel.
	// TODO: Razzer's mechanism: it considers only two calls.
	for c1 := 0; c1 < len(p.Calls); c1++ {
		for c2 := c1 + 1; c2 < len(p.Calls); c2++ {
			cont := prog.Contender{
				Calls: []int{c1, c2},
			}
			if fuzzer.checkMaxReadFrom(p, cont, info) {
				res = append(res, cont)
			}
			fuzzer.mergeMaxReadFrom(p, cont, info)
		}
	}
	return
}

func (fuzzer *Fuzzer) shutOffThreading(p *prog.Prog, calls prog.Contender, info *ipc.ProgInfo) bool {
	// Threading a given input requires at most O(n*n) re-execution to
	// collect read-from inside an epoch (i.e., conflicts), so the
	// threading queue may explode very quickly. To avoid that
	// situation, we shut off the threading work if
	if len(fuzzer.workQueue.threading) > maxWorkThreading {
		// 1) the threading workqueue already contains lots of
		// threading work. It is fine even if info contains
		// interesting read-froms. We don't lose a chance to find
		// threaded p because we don't collect the interesting
		// read-froms so we will eventually find similar threaded p in
		// the future.
		return true
	}
	return false
}

func signalPrio(p *prog.Prog, info *ipc.CallInfo, call int) (prio uint8) {
	if call == -1 {
		return 0
	}
	if info.Errno == 0 {
		prio |= 1 << 1
	}
	if !p.Target.CallContainsAny(p.Calls[call]) {
		prio |= 1 << 0
	}
	return
}

func parseOutputType(str string) OutputType {
	switch str {
	case "none":
		return OutputNone
	case "stdout":
		return OutputStdout
	case "dmesg":
		return OutputDmesg
	case "file":
		return OutputFile
	default:
		log.Fatalf("-output flag must be one of none/stdout/dmesg/file")
		return OutputNone
	}
}
