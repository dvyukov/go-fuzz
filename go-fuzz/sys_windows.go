// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
	"syscall"
	"unsafe"
)

func lowerProcessPrio() {
	// TODO: implement me
}

type Mapping struct {
	mapping syscall.Handle
	addr    uintptr
}

func createMapping(name string, size int) (*Mapping, []byte) {
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		log.Fatalf("failed to open comm file: %v", err)
	}
	defer f.Close()
	mapping, err := syscall.CreateFileMapping(syscall.InvalidHandle, nil, syscall.PAGE_READWRITE, 0, uint32(size), nil)
	if err != nil {
		log.Fatalf("failed to create file mapping: %v", err)
	}
	const FILE_MAP_ALL_ACCESS = 0xF001F
	addr, err := syscall.MapViewOfFile(mapping, FILE_MAP_ALL_ACCESS, 0, 0, uintptr(size))
	if err != nil {
		log.Fatalf("failed to mmap comm file: %v", err)
	}
	hdr := reflect.SliceHeader{addr, size, size}
	mem := *(*[]byte)(unsafe.Pointer(&hdr))
	mem[0] = 1 // test access
	return &Mapping{mapping, addr}, mem
}

func (m *Mapping) destroy() {
	syscall.UnmapViewOfFile(m.addr)
	syscall.CloseHandle(m.mapping)
}

func setupCommMapping(cmd *exec.Cmd, comm *Mapping, rOut, wIn *os.File) {
	syscall.SetHandleInformation(syscall.Handle(comm.mapping), syscall.HANDLE_FLAG_INHERIT, 1)
	syscall.SetHandleInformation(syscall.Handle(rOut.Fd()), syscall.HANDLE_FLAG_INHERIT, 1)
	syscall.SetHandleInformation(syscall.Handle(wIn.Fd()), syscall.HANDLE_FLAG_INHERIT, 1)
	cmd.Env = append(cmd.Env, fmt.Sprintf("GO_FUZZ_COMM_FD=%v", comm.mapping))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GO_FUZZ_IN_FD=%v", rOut.Fd()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GO_FUZZ_OUT_FD=%v", wIn.Fd()))
}
