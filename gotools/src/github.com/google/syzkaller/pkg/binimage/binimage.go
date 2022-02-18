package binimage

// NOTE: it seems Syzkaller already implements this functionality

import (
	"debug/dwarf"
	"debug/elf"
	"os"
	"sort"

	"github.com/google/syzkaller/pkg/log"
)

type BinaryImage struct {
	/* not used anyway */
	_elf   *elf.File
	_dwarf *dwarf.Data

	// dwarf reader
	reader *dwarf.Reader

	*elf.Section
	symbols  []elf.Symbol
	symToDir map[elf.Symbol]string
	// address of __sanitizer_cov_trace_pc
	kcov uint64
}

func BuildBinaryImage(image string) *BinaryImage {
	f, err := os.Open(image)
	if err != nil {
		panic(err)
	}
	_elf, err := elf.NewFile(f)
	if err != nil {
		panic(err)
	}
	return buildBinaryImage(_elf)
}

func buildBinaryImage(_elf *elf.File) *BinaryImage {
	if _elf.Class.String() != "ELFCLASS64" || _elf.Machine.String() != "EM_X86_64" {
		log.Fatalf("only support x86_64")
		/* not reachable */
		return nil
	}

	text := _elf.Section(".text")
	symbols, err := _elf.Symbols()
	if err != nil {
		panic("err")
	}

	_dwarf, err := _elf.DWARF()
	var reader *dwarf.Reader
	if err != nil {
		log.Logf(0, "[WARN] Failed to extract the dwarf info")
		_dwarf = nil
	} else {
		reader = _dwarf.Reader()
	}

	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].Value < symbols[j].Value
	})

	kcov := uint64(0)
	for _, sym := range symbols {
		if sym.Name == KCOV_FUNCNAME {
			kcov = sym.Value
		}
	}

	symToDir := make(map[elf.Symbol]string)

	return &BinaryImage{
		_elf:     _elf,
		_dwarf:   _dwarf,
		reader:   reader,
		Section:  text,
		symbols:  symbols,
		symToDir: symToDir,
		kcov:     kcov,
	}
}

func (bin *BinaryImage) Function(addr uint64) elf.Symbol {
	idx := sort.Search(len(bin.symbols), func(i int) bool {
		return bin.symbols[i].Value >= addr
	})

	if idx >= len(bin.symbols) {
		// Something wrong.
		return elf.Symbol{}
	}

	if bin.symbols[idx].Value != addr {
		idx -= 1
	}
	return bin.symbols[idx]
}

func (bin *BinaryImage) FileFromAddr(addr uint64) string {
	return fileFromAddr(bin.reader, addr)
}

func fileFromAddr(reader *dwarf.Reader, addr uint64) string {
	if reader == nil {
		return ""
	}

	// NOTE: SeekPC is slow. See the comments of SeekPC().
	e, err := reader.SeekPC(addr)
	if err != nil {
		return ""
	}

	f, ok := e.Val(dwarf.AttrName).(string)
	if !ok {
		return ""
	}

	return f
}

const KCOV_FUNCNAME = "__sanitizer_cov_trace_pc"