package scheduler

import (
	"sync"

	"github.com/google/syzkaller/pkg/interleaving"
)

type KnotterOpts struct {
	Signal *interleaving.Signal
	Mu     *sync.RWMutex
	Flags  KnotterFlags
}

type KnotterFlags int

const (
	FlagReassignThreadID KnotterFlags = 1 << iota
	FlagStrictTimestamp
	FlagWantParallel
)
