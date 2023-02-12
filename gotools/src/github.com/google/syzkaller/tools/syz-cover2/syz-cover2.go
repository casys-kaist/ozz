// Copyright 2018 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/syzkaller/pkg/interleaving"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/tool"
)

func main() {
	var (
		flagCoverageDir = flag.String("coverdir", "", "coverdir")
	)
	defer tool.Init()()

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "usage: syz-cover2 [flags] rawcover.file\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Logf(0, "reading coverage...")

	codecov, commscov, knotcov, err := readCoverage(*flagCoverageDir)
	if err != nil {
		tool.Fail(err)
	}

	log.Logf(0, "reading coverage... done.")

	err = checkCoverage(flag.Args(), codecov, commscov, knotcov)
	if err != nil {
		tool.Fail(err)
	}
}

func readCoverage(covdir string) ([]uint32, interleaving.Signal, interleaving.Signal, error) {
	const (
		codefn  = "code"
		commsfn = "communication"
		knotfn  = "knot"
	)

	data, err := ioutil.ReadFile(filepath.Join(covdir, commsfn))
	if err != nil {
		return nil, nil, nil, err
	}
	var commcov interleaving.Signal
	commcov.FromHex(data)

	data, err = ioutil.ReadFile(filepath.Join(covdir, knotfn))
	if err != nil {
		return nil, nil, nil, err
	}
	var knotcov interleaving.Signal
	knotcov.FromHex(data)

	return nil, commcov, knotcov, nil
}

func checkCoverage(files []string, codecov []uint32, commscov interleaving.Signal, knotcov interleaving.Signal) error {
	const (
		typComm = "communication"
		typKnot = "knot"
	)

	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}

		var comms []interleaving.Communication
		s := bufio.NewScanner(bytes.NewReader(data))
		s.Scan()
		typ := s.Text()
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			comm, err := stringToCommunication(line)
			if err != nil {
				return err
			}
			comms = append(comms, comm)
		}

		var ok bool
		var hsh uint64
		if strings.HasPrefix(typ, typComm) {
			hsh, ok = checkInterleaving(comms[0], commscov)
		} else if strings.HasPrefix(typ, typKnot) {
			seg := interleaving.Knot{comms[0], comms[1]}
			hsh, ok = checkInterleaving(seg, knotcov)
		} else {
			return errUnknownType
		}

		if ok {
			log.Logf(0, "Coverage %x from %s: found", hsh, file)
		} else {
			log.Logf(0, "Coverage %x from %s: not found", hsh, file)
		}
	}
	return nil
}

func checkInterleaving(int interleaving.Segment, cov interleaving.Signal) (uint64, bool) {
	hsh := int.Hash()
	_, ok := cov[hsh]
	return hsh, ok
}

func stringToCommunication(s string) (interleaving.Communication, error) {
	// Each line contains four numbers, addr1, timestamp1, addr2,
	// timestamp2
	var nums []uint64
	fields := strings.Fields(s)
	for _, f := range fields {
		num, err := strconv.ParseUint(f, 0, 64)
		if err != nil {
			return interleaving.Communication{}, err
		}
		nums = append(nums, num)
	}
	if len(nums) != 4 {
		return interleaving.Communication{}, errWrongFormat
	}
	return interleaving.Communication{
		interleaving.Access{Inst: uint32(nums[0]), Timestamp: uint32(nums[1])},
		interleaving.Access{Inst: uint32(nums[2]), Timestamp: uint32(nums[3])},
	}, nil
}

var (
	errUnknownType = fmt.Errorf("")
	errWrongFormat = fmt.Errorf("")
)
