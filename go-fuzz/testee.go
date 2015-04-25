package main

import (
	"encoding/binary"
	"log"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// Testee is a wrapper around one testee subprocess.
// It manages communication with the testee, timeouts and output collection.
type Testee struct {
	coverRegion []byte
	inputRegion []byte
	cmd         *exec.Cmd
	inPipe      *os.File
	outPipe     *os.File
	stdoutPipe  *os.File
	startTime   int64
	execs       int
	outputC     chan []byte
	downC       chan bool
	down        bool
}

func newTestee(bin, commFile string, coverRegion, inputRegion []byte) *Testee {
retry:
	rIn, wIn, err := os.Pipe()
	if err != nil {
		log.Fatalf("failed to pipe: %v", err)
	}
	rOut, wOut, err := os.Pipe()
	if err != nil {
		log.Fatalf("failed to pipe: %v", err)
	}
	rStdout, wStdout, err := os.Pipe()
	if err != nil {
		log.Fatalf("failed to pipe: %v", err)
	}
	comm, err := os.OpenFile(commFile, os.O_RDWR, 0)
	if err != nil {
		log.Fatalf("failed to open comm file: %v", err)
	}
	cmd := exec.Command(bin)
	cmd.Stdout = wStdout
	cmd.Stderr = wStdout
	cmd.ExtraFiles = append(cmd.ExtraFiles, comm)
	cmd.ExtraFiles = append(cmd.ExtraFiles, rOut)
	cmd.ExtraFiles = append(cmd.ExtraFiles, wIn)
	if err = cmd.Start(); err != nil {
		// This can be a transient failure like "cannot allocate memory" or "text file is busy".
		log.Printf("failed to start test binary: %v", err)
		rIn.Close()
		wIn.Close()
		rOut.Close()
		wOut.Close()
		rStdout.Close()
		wStdout.Close()
		comm.Close()
		time.Sleep(time.Second)
		goto retry
	}
	rOut.Close()
	wIn.Close()
	wStdout.Close()
	comm.Close()
	t := &Testee{
		coverRegion: coverRegion,
		inputRegion: inputRegion,
		cmd:         cmd,
		inPipe:      rIn,
		outPipe:     wOut,
		stdoutPipe:  rStdout,
		outputC:     make(chan []byte),
		downC:       make(chan bool),
	}
	// Stdout reader goroutine.
	go func() {
		// The testee should not output unless it crashes.
		// But there are still chances that it does. If so, it can overflow
		// the stdout pipe during testing and deadlock. To prevent the
		// deadlock we periodically read out stdout.
		// This goroutine also collects crash output.
		ticker := time.NewTicker(time.Second)
		const N = 1 << 20
		data := make([]byte, N)
		filled := 0
		for {
			select {
			case <-ticker.C:
			case <-t.downC:
			}
			n, err := t.stdoutPipe.Read(data[filled:])
			if err != nil {
				break
			}
			if *flagV >= 3 {
				log.Printf("testee: %v\n", string(data[filled:filled+n]))
			}
			filled += n
			if filled > N/4*3 {
				copy(data, data[N/2:filled])
				filled -= N / 2
			}
		}
		ticker.Stop()
		trimmed := make([]byte, filled)
		copy(trimmed, data)
		t.outputC <- trimmed
	}()
	// Hang watcher goroutine.
	go func() {
		timeout := time.Duration(*flagTimeout) * time.Millisecond
		ticker := time.NewTicker(timeout / 2)
		for {
			select {
			case <-ticker.C:
				start := atomic.LoadInt64(&t.startTime)
				if start != 0 && time.Now().UnixNano()-start > int64(timeout) {
					atomic.StoreInt64(&t.startTime, -1)
					t.cmd.Process.Signal(syscall.SIGABRT)
					time.Sleep(time.Second)
					t.cmd.Process.Signal(syscall.SIGKILL)
					ticker.Stop()
					return
				}
			case <-t.downC:
				ticker.Stop()
				return
			}

		}
	}()
	// Shutdown watcher goroutine.
	go func() {
		select {
		case <-t.downC:
		case <-shutdownC:
			t.cmd.Process.Signal(syscall.SIGKILL)
		}
	}()
	return t
}

// test passes data for testing.
func (t *Testee) test(data []byte) (res int, ns uint64, cover []byte, crashed, hanged, retry bool) {
	if t.down {
		log.Fatalf("cannot test: testee is already shutdown")
	}

	// The test binary can accumulate significant amount of memory,
	// so we recreate it periodically.
	t.execs++
	if t.execs > 10000 {
		t.cmd.Process.Signal(syscall.SIGKILL)
		retry = true
		return
	}

	for i := range t.coverRegion {
		t.coverRegion[i] = 0
	}
	copy(t.inputRegion[:], data)
	atomic.StoreInt64(&t.startTime, time.Now().UnixNano())
	if err := binary.Write(t.outPipe, binary.LittleEndian, uint64(len(data))); err != nil {
		if *flagV >= 1 {
			log.Printf("write to testee failed: %v", err)
		}
		retry = true
		return
	}
	// Once we do the write, the test is running.
	// Once we read the reply below, the test is done.
	type Reply struct {
		Res uint64
		Ns  uint64
	}
	var r Reply
	err := binary.Read(t.inPipe, binary.LittleEndian, &r)
	hanged = atomic.LoadInt64(&t.startTime) == -1
	atomic.StoreInt64(&t.startTime, 0)
	if err != nil || hanged {
		// Should have been crashed.
		crashed = true
		return
	}
	res = int(r.Res)
	ns = r.Ns
	cover = t.coverRegion
	return
}

func (t *Testee) shutdown() (output []byte) {
	if t.down {
		log.Fatalf("cannot shutdown: testee is already shutdown")
	}
	t.down = true
	t.cmd.Process.Kill() // it is probably already dead, but kill it again to be sure
	close(t.downC)       // wakeup stdout reader
	out := <-t.outputC
	t.cmd.Wait()
	t.inPipe.Close()
	t.outPipe.Close()
	t.stdoutPipe.Close()
	return out
}
