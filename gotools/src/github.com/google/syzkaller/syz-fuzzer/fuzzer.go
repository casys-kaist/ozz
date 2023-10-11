// Copyright 2015 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"
	golog "log"
	"math/rand"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/syzkaller/pkg/csource"
	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/host"
	"github.com/google/syzkaller/pkg/interleaving"
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
	name        string
	outputType  OutputType
	config      *ipc.Config
	execOpts    *ipc.ExecOpts
	procs       []*Proc
	gate        *ipc.Gate
	workQueue   *WorkQueue
	needPoll    chan struct{}
	choiceTable *prog.ChoiceTable
	collection  [CollectionCount]uint64
	noMutate    map[int]bool
	// The stats field cannot unfortunately be just an uint64 array, because it
	// results in "unaligned 64-bit atomic operation" errors on 32-bit platforms.
	stats             []uint64
	manager           *rpctype.RPCClient
	target            *prog.Target
	triagedCandidates uint32
	timeouts          targets.Timeouts
	shifter           map[uint32]uint32

	faultInjectionEnabled    bool
	comparisonTracingEnabled bool
	fetchRawCover            bool
	randomReordering         bool

	corpusMu        sync.RWMutex
	corpus          []*prog.Prog
	corpusHashes    map[hash.Sig]struct{}
	corpusPrios     []int64
	sumPrios        int64
	concurrentCalls []*prog.ConcurrentCalls

	signalMu     sync.RWMutex
	corpusSignal signal.Signal // signal of inputs in corpus
	maxSignal    signal.Signal // max signal ever observed including flakes
	newSignal    signal.Signal // diff of maxSignal since last sync with master

	// We manage additional interleaving signals. All semantics are
	// same as the ones for the code coverage.
	corpusInterleaving interleaving.Signal
	maxInterleaving    interleaving.Signal
	newInterleaving    interleaving.Signal

	instCount     map[uint32]uint32
	instBlacklist map[uint32]struct{}

	// Mostly for debugging scheduling mutation. If generate is false,
	// procs do not generate/mutate inputs but schedule.
	generate bool
	schedule bool

	m monitor

	checkResult *rpctype.CheckArgs
	logMu       sync.Mutex
}

type FuzzerSnapshot struct {
	corpus          []*prog.Prog
	concurrentCalls []*prog.ConcurrentCalls
	corpusPrios     []int64
	sumPrios        int64
	fuzzer          *Fuzzer
}

type Collection int

const (
	// Stats of collected data
	CollectionScheduleHint Collection = iota
	CollectionThreadingHint
	CollectionConcurrentCalls
	CollectionWQTriage
	CollectionWQTriageCandidate
	CollectionWQCandidate
	CollectionWQSmash
	CollectionWQThreading
	CollectionDurationTriage
	CollectionDurationCandidate
	CollectionDurationSmash
	CollectionDurationThreading
	CollectionDurationGen
	CollectionDurationFuzz
	CollectionDurationSchedule
	CollectionDurationCalc1
	CollectionDurationCalc2
	CollectionDurationTotal
	CollectionCount
)

var collectionNames = [CollectionCount]string{
	CollectionScheduleHint:      "schedule hint",
	CollectionThreadingHint:     "threading hint",
	CollectionConcurrentCalls:   "concurrent calls",
	CollectionWQTriage:          "workqueue triage",
	CollectionWQTriageCandidate: "workqueue triage candidate",
	CollectionWQCandidate:       "workqueue candidate",
	CollectionWQSmash:           "workqueue smash",
	CollectionWQThreading:       "workqueue threading",
	CollectionDurationTriage:    "duration triage",
	CollectionDurationCandidate: "duration candidate",
	CollectionDurationSmash:     "duration smash",
	CollectionDurationThreading: "duration threading",
	CollectionDurationGen:       "duration gen",
	CollectionDurationFuzz:      "duration fuzz",
	CollectionDurationSchedule:  "duration schedule",
	CollectionDurationCalc1:     "duration calc1",
	CollectionDurationCalc2:     "duration calc2",
	CollectionDurationTotal:     "duration total",
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
	StatCollide
	StatBufferTooSmall
	StatThreading
	StatSchedule
	StatThreadWorkTimeout
	StatCount
)

var statNames = [StatCount]string{
	StatGenerate:          "exec gen",
	StatFuzz:              "exec fuzz",
	StatCandidate:         "exec candidate",
	StatTriage:            "exec triage",
	StatMinimize:          "exec minimize",
	StatSmash:             "exec smash",
	StatHint:              "exec hints",
	StatSeed:              "exec seeds",
	StatCollide:           "exec collide",
	StatThreading:         "exec threadings",
	StatSchedule:          "exec schedulings",
	StatBufferTooSmall:    "buffer too small",
	StatThreadWorkTimeout: "threading work timeout",
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
	if features[host.FeatureDelayKcovMmap].Enabled {
		config.Flags |= ipc.FlagDelayKcovMmap
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
	if features[host.FeatureNicVF].Enabled {
		config.Flags |= ipc.FlagEnableNicVF
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

	var (
		flagName             = flag.String("name", "test", "unique name for manager")
		flagOS               = flag.String("os", runtime.GOOS, "target OS")
		flagArch             = flag.String("arch", runtime.GOARCH, "target arch")
		flagManager          = flag.String("manager", "", "manager rpc address")
		flagProcs            = flag.Int("procs", 1, "number of parallel test processes")
		flagOutput           = flag.String("output", "stdout", "write programs to none/stdout/dmesg/file")
		flagTest             = flag.Bool("test", false, "enable image testing mode")      // used by syz-ci
		flagRunTest          = flag.Bool("runtest", false, "enable program testing mode") // used by pkg/runtest
		flagRawCover         = flag.Bool("raw_cover", false, "fetch raw coverage")
		flagGen              = flag.Bool("gen", true, "generate/mutate inputs")
		flagShifter          = flag.String("shifter", "./shifter", "path to the shifter")
		flagRandomReordering = flag.Bool("random-reordering", false, "")
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
	if *flagRawCover {
		execOpts.Flags &^= ipc.FlagDedupCover
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
		log.Fatalf("failed to create an RPC client: %v ", err)
	}

	log.Logf(1, "connecting to manager...")
	a := &rpctype.ConnectArgs{
		Name:        *flagName,
		MachineInfo: machineInfo,
		Modules:     modules,
	}
	r := &rpctype.ConnectRes{}
	if err := manager.Call("Manager.Connect", a, r); err != nil {
		log.Fatalf("failed to call Manager.Connect(): %v ", err)
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

	shifter := readShifter(*flagShifter)

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
		shifter:                  shifter,

		corpusInterleaving: make(interleaving.Signal),
		maxInterleaving:    make(interleaving.Signal),
		newInterleaving:    make(interleaving.Signal),

		instCount:     make(map[uint32]uint32),
		instBlacklist: make(map[uint32]struct{}),

		checkResult: r.CheckResult,
		generate:    *flagGen,

		fetchRawCover:    *flagRawCover,
		randomReordering: *flagRandomReordering,
		noMutate:         r.NoMutateCalls,
		stats:            make([]uint64, StatCount),
	}
	gateCallback := fuzzer.useBugFrames(r, *flagProcs)
	fuzzer.gate = ipc.NewGate(2**flagProcs, gateCallback)

	for needCandidates, more := true, true; more; needCandidates = false {
		more = fuzzer.poll(needCandidates, nil, nil)
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

func readShifter(shifterPath string) map[uint32]uint32 {
	if shifter, err := __readShifter(shifterPath); err != nil {
		log.Logf(0, "Failed to read shifter: %v", err)
		return nil
	} else {
		return shifter
	}
}

// XXX: copied from the binimage package. We cannot import binimage
// since it requires libcapstone.
func __readShifter(path string) (map[uint32]uint32, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)

	var shifter map[uint32]uint32
	err = decoder.Decode(&shifter)
	if err != nil {
		return nil, err
	}

	return shifter, nil
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
			collections := make(map[string]uint64)
			for collection := Collection(0); collection < CollectionCount; collection++ {
				name := fuzzer.name + "-" + collectionNames[collection]
				v := atomic.LoadUint64(&fuzzer.collection[collection])
				collections[name] = v
			}
			for c, v := range fuzzer.m.get() {
				name := fuzzer.name + "-" + collectionNames[c]
				collections[name] = v / 1000 / 1000 / 1000 // ns -> s
			}
			if !fuzzer.poll(needCandidates, stats, collections) {
				lastPoll = time.Now()
			}
		}
	}
}

func (fuzzer *Fuzzer) serializeInstCount(instCount *map[uint32]uint32) []uint32 {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	ret := make([]uint32, 0, len(*instCount)*2)
	for k, v := range *instCount {
		ret = append(ret, k, v)
	}
	*instCount = make(map[uint32]uint32)
	return ret
}

func (fuzzer *Fuzzer) poll(needCandidates bool, stats, collections map[string]uint64) bool {
	start := time.Now()
	defer func() {
		log.Logf(0, "Poll takes %v", time.Since(start))
	}()
	a := &rpctype.PollArgs{
		Name:            fuzzer.name,
		NeedCandidates:  needCandidates,
		MaxSignal:       fuzzer.grabNewSignal().Serialize(),
		MaxInterleaving: fuzzer.grabNewInterleaving().Serialize(),
		Stats:           stats,
		Collections:     collections,
		InstCount:       fuzzer.serializeInstCount(&fuzzer.instCount),
	}
	if len(fuzzer.instCount) != 0 {
		panic("wrong")
	}

	r := &rpctype.PollRes{}
	if err := fuzzer.manager.Call("Manager.Poll", a, r); err != nil {
		log.Fatalf("Manager.Poll call failed: %v", err)
	}
	maxSignal := r.MaxSignal.Deserialize()
	maxInterleaving := r.MaxInterleaving.Deserialize()
	log.Logf(1, "poll: candidates=%v inputs=%v signal=%v interleaving=%v",
		len(r.Candidates), len(r.NewInputs), maxSignal.Len(), maxInterleaving.Len())

	const phaseTriagedCorpus = 2
	if r.ManagerPhase >= phaseTriagedCorpus {
		fuzzer.schedule = true
	}

	fuzzer.signalMu.Lock()
	for _, inst := range r.InstBlacklist {
		fuzzer.instBlacklist[inst] = struct{}{}
	}
	fuzzer.signalMu.Unlock()

	fuzzer.addMaxSignal(maxSignal)
	fuzzer.addMaxInterleaving(maxInterleaving)
	for _, inp := range r.NewInputs {
		fuzzer.addInputFromAnotherFuzzer(inp)
	}
	for _, candidate := range r.Candidates {
		fuzzer.addCandidateInput(candidate)
	}
	if needCandidates && len(r.Candidates) == 0 && atomic.LoadUint32(&fuzzer.triagedCandidates) == 0 {
		atomic.StoreUint32(&fuzzer.triagedCandidates, 1)
	}
	return len(r.NewInputs) != 0 || len(r.Candidates) != 0 || maxSignal.Len() != 0
}

func (fuzzer *Fuzzer) sendInputToManager(inp rpctype.Input) {
	a := &rpctype.NewInputArgs{
		Name:  fuzzer.name,
		Input: inp,
	}
	if err := fuzzer.manager.Call("Manager.NewInput", a, nil); err != nil {
		log.Fatalf("Manager.NewInput call failed: %v", err)
	}
}

func (fuzzer *Fuzzer) sendScheduledInputToManager(inp rpctype.ScheduledInput) {
	a := &rpctype.NewScheduledInputArgs{
		Name:           fuzzer.name,
		ScheduledInput: inp,
	}
	if err := fuzzer.manager.Call("Manager.NewScheduledInput", a, nil); err != nil {
		log.Fatalf("Manager.NewScheduledInput call failed: %v", err)
	}
}

func (fuzzer *Fuzzer) addInputFromAnotherFuzzer(inp rpctype.Input) {
	p := fuzzer.deserializeInput(inp.Prog)
	if p == nil {
		return
	}
	sig := hash.Hash(inp.Prog)
	sign := inp.Signal.Deserialize()
	fuzzer.addInputToCorpus(p, sign, sig)
}

func (fuzzer *Fuzzer) addCandidateInput(candidate rpctype.Candidate) {
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
	fuzzer.collectionWorkqueue(fuzzer.workQueue.stats())
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

func (fuzzer *FuzzerSnapshot) chooseThreadedProgram(r *rand.Rand) *prog.ConcurrentCalls {
	// TODO: Prioritize inputs according to the number of
	// hints.
	for retry := 0; len(fuzzer.concurrentCalls) != 0 && retry < 100; retry++ {
		idx := r.Intn(len(fuzzer.concurrentCalls))
		tp := fuzzer.concurrentCalls[idx]
		if len(tp.Hint) != 0 {
			return tp
		}
	}
	return nil
}

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

func (fuzzer *Fuzzer) bookScheduleGuide(p *prog.Prog, hint []interleaving.Segment) {
	log.Logf(2, "book a schedule guide")
	fuzzer.addCollection(CollectionScheduleHint, uint64(len(hint)))
	fuzzer.addCollection(CollectionConcurrentCalls, 1)
	fuzzer.corpusMu.Lock()
	defer fuzzer.corpusMu.Unlock()
	fuzzer.concurrentCalls = append(fuzzer.concurrentCalls, &prog.ConcurrentCalls{
		P:    p,
		Hint: hint,
	})
}

func (fuzzer *Fuzzer) addThreadedInputToCorpus(p *prog.Prog, sign interleaving.Signal) {
	// NOTE: We do not further mutate threaded prog so we do not add
	// it to corpus. This can be possibly limiting the fuzzer, but we
	// don't have any evidence of it.
	fuzzer.signalMu.Lock()
	fuzzer.maxInterleaving.Merge(sign)
	fuzzer.corpusInterleaving.Merge(sign)
	fuzzer.signalMu.Unlock()
}

func (fuzzer *Fuzzer) snapshot() FuzzerSnapshot {
	fuzzer.removeExhaustedConcurrentCalls()
	fuzzer.corpusMu.RLock()
	defer fuzzer.corpusMu.RUnlock()
	return FuzzerSnapshot{
		fuzzer.corpus,
		fuzzer.concurrentCalls,
		fuzzer.corpusPrios,
		fuzzer.sumPrios,
		fuzzer,
	}
}

func (fuzzer *Fuzzer) removeExhaustedConcurrentCalls() {
	fuzzer.corpusMu.Lock()
	removed := uint64(0)
	ln := len(fuzzer.concurrentCalls)
	for i := 0; i < ln; {
		c := fuzzer.concurrentCalls[i]
		if len(c.Hint) != 0 {
			i++
			continue
		}
		// remove the exhausted concurrent call
		fuzzer.concurrentCalls[i] = fuzzer.concurrentCalls[ln-1]
		fuzzer.concurrentCalls = fuzzer.concurrentCalls[:ln-1]
		ln--
	}
	fuzzer.corpusMu.Unlock()
	if removed != 0 {
		log.Logf(2, "removed %d concurrent calls", removed)
		fuzzer.subCollection(CollectionConcurrentCalls, removed)
	}
}

func (fuzzer *Fuzzer) addMaxSignal(sign signal.Signal) {
	if sign.Len() == 0 {
		return
	}
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	fuzzer.maxSignal.Merge(sign)
}

func (fuzzer *Fuzzer) addMaxInterleaving(sign interleaving.Signal) {
	if sign.Len() == 0 {
		return
	}
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	fuzzer.maxInterleaving.Merge(sign)
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

func (fuzzer *Fuzzer) grabNewInterleaving() interleaving.Signal {
	fuzzer.signalMu.Lock()
	defer fuzzer.signalMu.Unlock()
	sign := fuzzer.newInterleaving
	if sign.Empty() {
		return nil
	}
	fuzzer.newInterleaving = nil
	return sign
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

func (fuzzer *Fuzzer) newSegment(base *interleaving.Signal, segs []interleaving.Segment) []interleaving.Segment {
	fuzzer.signalMu.RLock()
	defer fuzzer.signalMu.RUnlock()
	return base.DiffRaw(segs)
}

func (fuzzer *Fuzzer) getNewKnot(knots []interleaving.Segment) []interleaving.Segment {
	diff := fuzzer.newSegment(&fuzzer.maxInterleaving, knots)
	if len(diff) == 0 {
		return nil
	}
	sign := interleaving.FromCoverToSignal(diff)
	fuzzer.signalMu.Lock()
	fuzzer.newInterleaving.Merge(sign)
	fuzzer.maxInterleaving.Merge(sign)
	fuzzer.signalMu.Unlock()
	return diff
}

func (fuzzer *Fuzzer) shutOffThreading(p *prog.Prog) bool {
	const maxThreadingKnots = 500000
	// So the threading queue may explode very quickly when starting a
	// fuzzer. To prevent the OOM killer, we shut off the threading
	// work if the threading queue already contains a lot of Knots
	fuzzer.corpusMu.RLock()
	threadingKnots := fuzzer.collection[CollectionThreadingHint]
	fuzzer.corpusMu.RUnlock()
	if threadingKnots > maxThreadingKnots {
		return true
	}
	return false
}

func (fuzzer *Fuzzer) addCollection(collection Collection, num uint64) {
	fuzzer.corpusMu.Lock()
	defer fuzzer.corpusMu.Unlock()
	fuzzer.collection[collection] += num
	log.Logf(2, "add %d collection to %s, total=%d",
		num,
		collectionNames[collection],
		fuzzer.collection[collection])
}

func (fuzzer *Fuzzer) subCollection(collection Collection, num uint64) {
	fuzzer.corpusMu.Lock()
	defer fuzzer.corpusMu.Unlock()
	fuzzer.collection[collection] -= num
	log.Logf(2, "sub %d collection to %s, total=%d",
		num,
		collectionNames[collection],
		fuzzer.collection[collection])
}

func (fuzzer *Fuzzer) countInstructionInKnot(knot interleaving.Knot) {
	for _, comm := range knot {
		for _, acc := range comm {
			fuzzer.instCount[acc.Inst]++
		}
	}
}

func (fuzzer *Fuzzer) sendUsedKnots(used []interleaving.Segment) {
	// XXX: At this point, we knot that instructions in thread0 were
	// executed before instructions in thread1. As we want to track
	// instruction addresses only, flatten the knot in a simple way.
	a := rpctype.SendUsedKnotsArg{}
	// XXX: Also we know that len(used) == 1 for now.
	for _, knot0 := range used {
		knot, ok := knot0.(interleaving.Knot)
		if !ok {
			panic("wrong")
		}
		insts := []uint32{}
		for _, comm := range knot {
			insts = append(insts, comm.Former().Inst, comm.Latter().Inst)
		}
		a.Insts = append(a.Insts, insts)
	}
	if err := fuzzer.manager.Call("Manager.SendUsedKnots", a, nil); err != nil {
		log.Fatalf("failed to call Manager.Connect(): %v ", err)
	}
}

func (fuzzer *Fuzzer) collectionWorkqueue(tricand, cand, tri, smash, thr uint64) {
	fuzzer.corpusMu.Lock()
	defer fuzzer.corpusMu.Unlock()
	fuzzer.collection[CollectionWQTriageCandidate] = tricand
	fuzzer.collection[CollectionWQCandidate] = cand
	fuzzer.collection[CollectionWQTriage] = tri
	fuzzer.collection[CollectionWQSmash] = smash
	fuzzer.collection[CollectionWQThreading] = thr
}

func (fuzzer *Fuzzer) canSchedule() bool {
	return fuzzer.schedule
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
