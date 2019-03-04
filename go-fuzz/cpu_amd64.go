// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

// Adapted from GOROOT/src/internal/cpu/cpu_x86.go.

var hasAVX2 bool

func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
func xgetbv() (eax, edx uint32)

const (
	// ecx bits
	cpuid_OSXSAVE = 1 << 27

	// ebx bits
	cpuid_AVX2 = 1 << 5
)

func init() {
	_, _, ecx1, _ := cpuid(1, 0)
	hasOSXSAVE := cpuBitIsSet(ecx1, cpuid_OSXSAVE)

	osSupportsAVX := false
	// For XGETBV, OSXSAVE bit is required and sufficient.
	if hasOSXSAVE {
		eax, _ := xgetbv()
		// Check if XMM and YMM registers have OS support.
		osSupportsAVX = cpuBitIsSet(eax, 1<<1) && cpuBitIsSet(eax, 1<<2)
	}

	_, ebx7, _, _ := cpuid(7, 0)
	hasAVX2 = cpuBitIsSet(ebx7, cpuid_AVX2) && osSupportsAVX
}

func cpuBitIsSet(hwc uint32, value uint32) bool {
	return hwc&value != 0
}
