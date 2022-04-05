package binimage

// NOTE: it seems Syzkaller already implements this functionality

import (
	"debug/dwarf"
	"debug/elf"
	"os"
	"sort"

	"github.com/knightsc/gapstone"

	"github.com/google/syzkaller/pkg/log"
)

type BinaryImage struct {
	workdir string
	image   string

	_elf *elf.File
	/* not used anyway */
	_dwarf *dwarf.Data

	// dwarf reader
	reader *dwarf.Reader

	engine gapstone.Engine

	*elf.Section
	symbols []elf.Symbol
	// address of __sanitizer_cov_trace_pc
	kcov uint64
	// address of sanitize_memcov_trace_load
	kmemcovLoad uint64
	// address of sanitize_memcov_trace_store
	kmemcovStore uint64

	shifter map[uint32]uint32
}

func BuildBinaryImage(workdir, image string) (*BinaryImage, error) {
	f, err := os.Open(image)
	if err != nil {
		return nil, err
	}
	_elf, err := elf.NewFile(f)
	if err != nil {
		return nil, err
	}
	return buildBinaryImage(workdir, image, _elf)
}

func buildBinaryImage(workdir, image string, _elf *elf.File) (*BinaryImage, error) {
	if _elf.Class.String() != "ELFCLASS64" || _elf.Machine.String() != "EM_X86_64" {
		log.Fatalf("only support x86_64")
		/* not reachable */
		return nil, nil
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

	kcov, kmemcovLoad, kmemcovStore := uint64(0), uint64(0), uint64(0)
	for _, sym := range symbols {
		if sym.Name == KCOV_FUNCNAME {
			kcov = sym.Value
		} else if sym.Name == KMEMCOV_STORE_FUNCNAME {
			kmemcovStore = sym.Value
		} else if sym.Name == KMEMCOV_LOAD_FUNCNAME {
			kmemcovLoad = sym.Value
		}
	}

	engine, err := gapstone.New(
		gapstone.CS_ARCH_X86,
		gapstone.CS_MODE_64,
	)
	if err != nil {
		return nil, err
	}
	engine.SetOption(gapstone.CS_OPT_DETAIL, gapstone.CS_OPT_ON)

	return &BinaryImage{
		workdir:      workdir,
		image:        image,
		_elf:         _elf,
		_dwarf:       _dwarf,
		reader:       reader,
		Section:      text,
		symbols:      symbols,
		kcov:         kcov,
		kmemcovLoad:  kmemcovLoad,
		kmemcovStore: kmemcovStore,
		engine:       engine,
	}, nil
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

const (
	KCOV_FUNCNAME          = "__sanitizer_cov_trace_pc"
	KMEMCOV_STORE_FUNCNAME = "sanitize_memcov_trace_store"
	KMEMCOV_LOAD_FUNCNAME  = "sanitize_memcov_trace_load"
)
