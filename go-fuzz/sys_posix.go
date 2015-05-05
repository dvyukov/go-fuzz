// +build darwin linux

package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

type Mapping struct {
	f *os.File
}

func createMapping(name string) *Mapping {
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		log.Fatalf("failed to open comm file: %v", err)
	}
	return &Mapping{f}
}

func (m *Mapping) mmap(size int) []byte {
	mem, err := syscall.Mmap(int(m.f.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		log.Fatalf("failed to mmap comm file: %v", err)
	}
	return mem
}

func (m *Mapping) destroy() {
	m.f.Close()
}

func setupCommMapping(cmd *exec.Cmd, comm *Mapping, rOut, wIn *os.File) {
	cmd.ExtraFiles = append(cmd.ExtraFiles, comm.f)
	cmd.ExtraFiles = append(cmd.ExtraFiles, rOut)
	cmd.ExtraFiles = append(cmd.ExtraFiles, wIn)
}
