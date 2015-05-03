package gofuzzdep

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

const (
	commFD = 3
	inFD   = 4
	outFD  = 5
)

var (
	CoverTab    *[CoverSize]byte
	input       []byte
	sonarRegion []byte
	sonarPos    uint32
)

func init() {
	mem, err := syscall.Mmap(commFD, 0, CoverSize+MaxInputSize+SonarRegionSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		println("failed to mmap fd = 3 errno =", err.(syscall.Errno))
		syscall.Exit(1)
	}
	CoverTab = (*[CoverSize]byte)(unsafe.Pointer(&mem[0]))
	input = mem[CoverSize : CoverSize+MaxInputSize]
	sonarRegion = mem[CoverSize+MaxInputSize:]
}

func Main(f func([]byte) int) {
	runtime.GOMAXPROCS(1) // makes coverage more deterministic, we parallelize on higher level
	for {
		n := read(inFD)
		if n > uint64(len(input)) {
			println("invalid input length")
			syscall.Exit(1)
		}
		for i := range CoverTab {
			CoverTab[i] = 0
		}
		atomic.StoreUint32(&sonarPos, 0)
		t0 := time.Now()
		res := f(input[:n])
		ns := time.Since(t0)
		write(outFD, uint64(res), uint64(ns), uint64(atomic.LoadUint32(&sonarPos)))
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
	return deserialize64(buf[:])
}

// write writes little-endian-encoded vals... to fd.
func write(fd int, vals ...uint64) {
	var tmp [3 * 8]byte
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

// writeStr writes strings s to fd.
func writeStr(fd int, s string) {
	buf := []byte(s)
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
