package binimage

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/gob"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/osutil"
	"github.com/knightsc/gapstone"
)

func (bin *BinaryImage) BuildOrReadShifter() (map[uint]uint, string, []string, error) {
	hsh, err := osutil.BinaryHash(bin.image)
	if err != nil {
		log.Fatalf("%v", err)
	}
	filename := hex.EncodeToString(hsh)

	absPath := filepath.Join(bin.workdir, "shifter-"+filename)
	if osutil.IsExist(absPath) {
		shifter, err := ReadShifter(absPath)
		if err != nil {
			return nil, "", nil, err
		}
		return shifter, absPath, nil, nil
	} else {
		shifter, failed, err := bin.buildShifter()
		if err != nil {
			return nil, "", nil, err
		}
		err = WriteShifter(absPath, shifter)
		if err != nil {
			return nil, "", nil, err
		}
		return shifter, absPath, failed, nil
	}
}

func (bin *BinaryImage) buildShifter() (map[uint]uint, []string, error) {
	bin.shifter = make(map[uint]uint)

	text := bin._elf.Section(".text")
	data, err := text.Data()
	if err != nil {
		return nil, nil, err
	}

	failed := []string{}
	for _, sym := range bin.symbols {
		const symbolTypeFunc = 2
		typ := sym.Info & 0xf
		if typ != symbolTypeFunc {
			continue
		}
		offset := sym.Value - text.Addr
		size := sym.Size
		if offset < 0 || offset+size > uint64(len(data)) {
			// XXX: Symbols, for example, in .init.text and .exit.text
			// fall into this if-statement. Not sure we want to build
			// shifter for them. Since symbols are sorted according to
			// sym.Value, we can break the loop
			break
		}
		if err := bin.buildShifterForFunction(data[offset:offset+size], sym); err != nil {
			failed = append(failed, sym.Name)
		}
	}
	return bin.shifter, failed, nil
}

func (bin *BinaryImage) buildShifterForFunction(data []byte, sym elf.Symbol) error {
	insns, err := bin.engine.Disasm(data, sym.Value, 0)
	if err != nil {
		return err
	}

	type kmemcovCall struct {
		addr uint
		load bool
	}

	var kmemcovCallInst kmemcovCall

	for _, insn := range insns {
		if strings.HasPrefix(insn.Mnemonic, "call") {
			x86insn := insn.X86
			op0 := x86insn.Operands[0]
			if op0.Type != gapstone.X86_OP_IMM {
				// Ignore indirect calls. It is very unlikely that our
				// callbacks are indirectly called.
				continue
			}
			// insn.Address calls op0.Imm
			callee := uint64(op0.Imm)
			if callee == bin.kmemcovLoad || callee == bin.kmemcovStore {
				if kmemcovCallInst.addr != 0 {
					// This can be happend, for example, the variable
					// is assigned to a register. In those cases, we
					// don't need a shifter anyway. Let's print a
					// debug message for seeing there is another case.
					log.Logf(4, "Inst %x is not handled, %x", kmemcovCallInst.addr, insn.Address)
				}
				// We want the *return* address of the call
				// instruction
				kmemcovCallInst = kmemcovCall{
					addr: insn.Address + insn.Size,
					load: callee == bin.kmemcovLoad,
				}
			}
		} else if kmemcovCallInst.addr == 0 {
			continue
		} else if ok := memoryOperand(insn, kmemcovCallInst.load); ok {
			// We will install a breakpoint on the *next* instruction
			// of the memory accessing instruction
			dst := insn.Address + insn.Size
			bin.shifter[kmemcovCallInst.addr] = dst - kmemcovCallInst.addr
			kmemcovCallInst.addr = 0
		}
	}
	return nil
}

func memoryOperand(insn gapstone.Instruction, load bool) bool {
	if strings.HasPrefix(insn.Mnemonic, "lea") {
		return false
	}

	x86Inst := insn.X86
	var accType uint8
	if load {
		accType = gapstone.CS_AC_READ
	} else {
		accType = gapstone.CS_AC_WRITE
	}

	for _, op := range x86Inst.Operands {
		acc := op.Access
		if strings.Contains(insn.Mnemonic, "cmpxchg") {
			// XXX: Is this a capstone bug? Many (all?) of cmpxchg
			// have an operand that has op.Access as READ, is in fact
			// is READ|WRITE.
			acc = gapstone.CS_AC_READ | gapstone.CS_AC_WRITE
		}
		if op.Type == gapstone.X86_OP_MEM && (acc&accType) != 0 {
			return true
		}
	}
	return false
}

func ReadShifter(path string) (map[uint]uint, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)

	var shifter map[uint]uint
	err = decoder.Decode(&shifter)
	if err != nil {
		return nil, err
	}

	return shifter, nil
}

func WriteShifter(path string, shifter map[uint]uint) error {
	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)

	err := encoder.Encode(shifter)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	buf.WriteTo(w)

	return nil
}
