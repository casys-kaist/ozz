package main

import (
	"syscall"
	"unsafe"
)

func resetKSSB() {
	flushVector := []byte{byte(1)}
	flushVectorp := uintptr(unsafe.Pointer(&flushVector))
	syscall.Syscall(SYS_SSB_FEEDINPUT, flushVectorp, 1, 0)
}

const SYS_SSB_FEEDINPUT = 500
