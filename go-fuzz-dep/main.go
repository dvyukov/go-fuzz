package gofuzzdep

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	coverSize    = 64 << 10
	maxInputSize = 1 << 20

	commFD = 3
	inFD   = 4
	outFD  = 5
)

var (
	CoverTab *[coverSize]byte
	input    []byte
)

func init() {
	mem, err := syscall.Mmap(commFD, 0, coverSize+maxInputSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		println("failed to mmap fd = 3 errno =", err.(syscall.Errno))
		syscall.Exit(1)
	}
	CoverTab = (*[coverSize]byte)(unsafe.Pointer(&mem[0]))
	input = mem[coverSize:]
}

func Main(f func([]byte) int, lits string) {
	runtime.GOMAXPROCS(1) // makes coverage more deterministic, we parallelize on higher level
	for {
		n := read(inFD)
		if n > uint64(len(input)) {
			println("invalid input length")
			syscall.Exit(1)
		}
		t0 := time.Now()
		res := f(input[:n])
		ns := time.Since(t0)
		write(outFD, uint64(res), uint64(ns))
	}
}

// read reads little-endian-encoded uint64 from fd.
func read(fd int) uint64 {
	rd := 0
	var buf [8]byte
	for rd != len(buf) {
		n, err := syscall.Read(fd, buf[rd:])
		if err == syscall.EINTR {
			continue
		}
		if n == 0 {
			syscall.Exit(1)
		}
		if err != nil {
			println("failed to read fd =", fd, "errno =", err.(syscall.Errno))
			syscall.Exit(1)
		}
		rd += n
	}
	return uint64(buf[0])<<0 |
		uint64(buf[1])<<8 |
		uint64(buf[2])<<16 |
		uint64(buf[3])<<24 |
		uint64(buf[4])<<32 |
		uint64(buf[5])<<40 |
		uint64(buf[6])<<48 |
		uint64(buf[7])<<56
}

// write writes little-endian-encoded vals... to fd.
func write(fd int, vals ...uint64) {
	var tmp [2 * 8]byte
	buf := tmp[:len(vals)*8]
	for i, v := range vals {
		for j := 0; j < 8; j++ {
			buf[i*8+j] = byte(v)
			v >>= 8
		}
	}
	wr := 0
	for wr != len(buf) {
		n, err := syscall.Write(fd, buf[wr:])
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			println("failed to read fd =", fd, "errno =", err.(syscall.Errno))
			syscall.Exit(1)
		}
		wr += n
	}
}
