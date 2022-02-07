package binimage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildTestBinImage(t *testing.T) *BinaryImage {
	testImage := filepath.Join("testdata", "a.out")
	if !fileExists(testImage) {
		t.Fatalf("the binary image is missing")
		return nil
	}
	binimage := BuildBinaryImage(testImage)
	if binimage == nil {
		// Do not have the test image
		t.Fatalf("failed to build the binary image")
		return nil
	}
	return binimage
}

func TestFunction(t *testing.T) {
	binimage := buildTestBinImage(t)
	tests := []struct {
		addr uint64
		fn   string
	}{
		{0x40054c, "main"},
		{0x40056b, "main"},
		{0x400570, "__libc_csu_init"},
		{0x4004f0, "frame_dummy"},
		{0x4004f2, "foo"},
		{0x400503, "foo"},
		{0x400530, "foo"},
		{0x400531, "bar"},
	}
	for _, test := range tests {
		got := binimage.Function(test.addr)
		if got.Name != test.fn {
			t.Errorf("wrong result, expected=%v, got=%v", test.fn, got.Name)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func TestDirFromFunc(t *testing.T) {
	binimage := buildTestBinImage(t)
	tests := []uint64{0x400530 /*foo*/, 0x400531 /*bar*/}
	for _, test := range tests {
		f := binimage.FileFromAddr(test)
		if !strings.HasPrefix(f, "test.c") {
			t.Errorf("wrong filename %v", f)
		}
	}
}
