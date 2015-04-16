package gofuzzdep

import (
	"runtime"
	"syscall"
	"unsafe"
)

const (
	commFD = 3
	inFD   = 4
	outFD  = 5
)

var (
	CoverTab     *[64 << 10]byte
	fakeCoverTab [64 << 10]byte
	input        []byte
)

func init() {
	mem, err := syscall.Mmap(commFD, 0, 64<<10+1<<20, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		println("failed to mmap fd = 3 errno =", err.(syscall.Errno))
		syscall.Exit(1)
	}
	CoverTab = (*[64 << 10]byte)(unsafe.Pointer(&mem[0]))
	input = mem[64<<10:]
}

func Main(f func([]byte) int) {
	runtime.GOMAXPROCS(1) // makes coverage more deterministic, we parallelize on higher level
	for {
		n := read(inFD)
		if n > uint64(len(input)) {
			println("invalid input length")
			syscall.Exit(1)
		}
		res := f(input[:n])
		write(outFD, uint64(res))
	}
}

func read(fd int) uint64 {
	rd := 0
	var buf [8]byte
	for rd != len(buf) {
		n, err := syscall.Read(fd, buf[rd:])
		if err == syscall.EINTR {
			continue
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

func write(fd int, v uint64) {
	buf := [8]byte{
		byte(v >> 0),
		byte(v >> 8),
		byte(v >> 16),
		byte(v >> 24),
		byte(v >> 32),
		byte(v >> 40),
		byte(v >> 48),
		byte(v >> 56),
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
