// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

//go:build !go1.17
// +build !go1.17

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func setupCommMapping(cmd *exec.Cmd, comm *Mapping, rOut, wIn *os.File) {
	syscall.SetHandleInformation(syscall.Handle(comm.mapping), syscall.HANDLE_FLAG_INHERIT, 1)
	syscall.SetHandleInformation(syscall.Handle(rOut.Fd()), syscall.HANDLE_FLAG_INHERIT, 1)
	syscall.SetHandleInformation(syscall.Handle(wIn.Fd()), syscall.HANDLE_FLAG_INHERIT, 1)
	cmd.Env = append(cmd.Env, fmt.Sprintf("GO_FUZZ_COMM_FD=%v", comm.mapping))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GO_FUZZ_IN_FD=%v", rOut.Fd()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GO_FUZZ_OUT_FD=%v", wIn.Fd()))
}
