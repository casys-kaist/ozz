package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/google/syzkaller/pkg/ipc"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/prog"
)

func (proc *Proc) logProgram(opts *ipc.ExecOpts, p *prog.Prog) {
	if proc.fuzzer.outputType == OutputNone {
		return
	}

	data := p.Serialize()
	strOpts := ""
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
