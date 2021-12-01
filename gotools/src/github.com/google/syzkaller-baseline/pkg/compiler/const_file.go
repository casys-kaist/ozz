// Copyright 2020 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package compiler

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/syzkaller/pkg/ast"
)

// ConstFile serializes/deserializes .const files.
type ConstFile struct {
	arches map[string]bool
	m      map[string]constVal
}

type constVal struct {
	name string
	vals map[string]uint64 // arch -> value
}

const undefined = "???"

func NewConstFile() *ConstFile {
	return &ConstFile{
		arches: make(map[string]bool),
		m:      make(map[string]constVal),
	}
}

func (cf *ConstFile) AddArch(arch string, consts map[string]uint64, undeclared map[string]bool) error {
	cf.arches[arch] = true
	for name, val := range consts {
		if err := cf.addConst(arch, name, val, true); err != nil {
			return err
		}
	}
	for name := range undeclared {
		if err := cf.addConst(arch, name, 0, false); err != nil {
			return err
		}
	}
	return nil
}

func (cf *ConstFile) addConst(arch, name string, val uint64, declared bool) error {
	cv := cf.m[name]
	if cv.vals == nil {
		cv.name = name
		cv.vals = make(map[string]uint64)
	}
	if val0, declared0 := cv.vals[arch]; declared && declared0 && val != val0 {
		return fmt.Errorf("const=%v arch=%v has different values: %v[%v] vs %v[%v]",
			name, arch, val, declared, val0, declared0)
	}
	if declared {
		cv.vals[arch] = val
	}
	cf.m[name] = cv
	return nil
}

func (cf *ConstFile) Arch(arch string) map[string]uint64 {
	if cf == nil {
		return nil
	}
	m := make(map[string]uint64)
	for name, cv := range cf.m {
		if v, ok := cv.vals[arch]; ok {
			m[name] = v
		}
	}
	return m
}

func (cf *ConstFile) Serialize() []byte {
	if len(cf.arches) == 0 {
		return nil
	}
	var arches []string
	for arch := range cf.arches {
		arches = append(arches, arch)
	}
	sort.Strings(arches)
	var consts []constVal
	for _, cv := range cf.m {
		consts = append(consts, cv)
	}
	sort.Slice(consts, func(i, j int) bool {
		return consts[i].name < consts[j].name
	})
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "# Code generated by syz-sysgen. DO NOT EDIT.\n")
	fmt.Fprintf(buf, "arches = %v\n", strings.Join(arches, ", "))
	for _, cv := range consts {
		fmt.Fprintf(buf, "%v = ", cv.name)
		if len(cv.vals) == 0 {
			// Undefined for all arches.
			fmt.Fprintf(buf, "%v\n", undefined)
			continue
		}
		count := make(map[uint64]int)
		max, dflt := 0, uint64(0)
		for _, val := range cv.vals {
			count[val]++
			if count[val] > 1 && (count[val] > max || count[val] == max && val < dflt) {
				max, dflt = count[val], val
			}
		}
		if max != 0 {
			// Have a default value.
			fmt.Fprintf(buf, "%v", dflt)
		}
		handled := make([]bool, len(arches))
		for i, arch := range arches {
			val, ok := cv.vals[arch]
			if ok && (max != 0 && val == dflt) || handled[i] {
				// Default value or serialized on a previous iteration.
				continue
			}
			if i != 0 || max != 0 {
				fmt.Fprintf(buf, ", ")
			}
			fmt.Fprintf(buf, "%v:", arch)
			for j := i + 1; j < len(arches); j++ {
				// Add more arches with the same value.
				arch1 := arches[j]
				val1, ok1 := cv.vals[arch1]
				if ok1 == ok && val1 == val {
					fmt.Fprintf(buf, "%v:", arch1)
					handled[j] = true
				}
			}
			if ok {
				fmt.Fprintf(buf, "%v", val)
			} else {
				fmt.Fprint(buf, undefined)
			}
		}
		fmt.Fprintf(buf, "\n")
	}
	return buf.Bytes()
}

func DeserializeConstFile(glob string, eh ast.ErrorHandler) *ConstFile {
	if eh == nil {
		eh = ast.LoggingHandler
	}
	files, err := filepath.Glob(glob)
	if err != nil {
		eh(ast.Pos{}, fmt.Sprintf("failed to find const files: %v", err))
		return nil
	}
	if len(files) == 0 {
		eh(ast.Pos{}, fmt.Sprintf("no const files matched by glob %q", glob))
		return nil
	}
	cf := NewConstFile()
	oldFormat := regexp.MustCompile(`_([a-z0-9]+)\.const$`)
	for _, f := range files {
		data, err := ioutil.ReadFile(f)
		if err != nil {
			eh(ast.Pos{}, fmt.Sprintf("failed to read const file: %v", err))
			return nil
		}
		// Support for old per-arch format.
		// Remove it once we don't have any *_arch.const files anymore.
		arch := ""
		if match := oldFormat.FindStringSubmatch(f); match != nil {
			arch = match[1]
		}
		if !cf.deserializeFile(data, filepath.Base(f), arch, eh) {
			return nil
		}
	}
	return cf
}

func (cf *ConstFile) deserializeFile(data []byte, file, arch string, eh ast.ErrorHandler) bool {
	pos := ast.Pos{File: file, Line: 1}
	errf := func(msg string, args ...interface{}) bool {
		eh(pos, fmt.Sprintf(msg, args...))
		return false
	}
	s := bufio.NewScanner(bytes.NewReader(data))
	var arches []string
	for ; s.Scan(); pos.Line++ {
		line := s.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq == -1 {
			return errf("expect '='")
		}
		name, val := strings.TrimSpace(line[:eq]), strings.TrimSpace(line[eq+1:])
		if arch != "" {
			// Old format.
			if !cf.parseOldConst(arch, name, val, errf) {
				return false
			}
			continue
		}
		if arch == "" && len(arches) == 0 {
			if name != "arches" {
				return errf("missing arches header")
			}
			for _, arch := range strings.Split(val, ",") {
				arches = append(arches, strings.TrimSpace(arch))
			}
			continue
		}
		if !cf.parseConst(arches, name, val, errf) {
			return false
		}
	}
	if err := s.Err(); err != nil {
		return errf("failed to parse: %v", err)
	}
	return true
}

type errft func(msg string, args ...interface{}) bool

func (cf *ConstFile) parseConst(arches []string, name, line string, errf errft) bool {
	var dflt map[string]uint64
	for _, pair := range strings.Split(line, ",") {
		fields := strings.Split(pair, ":")
		if len(fields) == 1 {
			// Default value.
			if dflt != nil {
				return errf("duplicate default value")
			}
			dflt = make(map[string]uint64)
			valStr := strings.TrimSpace(fields[0])
			if valStr == undefined {
				continue
			}
			val, err := strconv.ParseUint(valStr, 0, 64)
			if err != nil {
				return errf("failed to parse int: %v", err)
			}
			for _, arch := range arches {
				dflt[arch] = val
			}
			continue
		}
		if len(fields) < 2 {
			return errf("bad value: %v", pair)
		}
		valStr := strings.TrimSpace(fields[len(fields)-1])
		defined := valStr != undefined
		var val uint64
		if defined {
			var err error
			if val, err = strconv.ParseUint(valStr, 0, 64); err != nil {
				return errf("failed to parse int: %v", err)
			}
		}
		for _, arch := range fields[:len(fields)-1] {
			arch = strings.TrimSpace(arch)
			delete(dflt, arch)
			if err := cf.addConst(arch, name, val, defined); err != nil {
				return errf("%v", err)
			}
		}
	}
	for arch, val := range dflt {
		if err := cf.addConst(arch, name, val, true); err != nil {
			return errf("%v", err)
		}
	}
	return true
}

func (cf *ConstFile) parseOldConst(arch, name, line string, errf errft) bool {
	val, err := strconv.ParseUint(strings.TrimSpace(line), 0, 64)
	if err != nil {
		return errf("failed to parse int: %v", err)
	}
	if err := cf.addConst(arch, name, val, true); err != nil {
		return errf("%v", err)
	}
	return true
}
