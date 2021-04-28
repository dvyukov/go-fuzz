// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

// Testee is a wrapper around one testee subprocess.
// It manages communication with the testee, timeouts and output collection.
type Testee struct {
	coverRegion []byte
	inputRegion []byte
	sonarRegion []byte
	cmd         *exec.Cmd
	inPipe      *os.File
	outPipe     *os.File
	stdoutPipe  *os.File
	writebuf    [9]byte  // reusable write buffer
	resbuf      [24]byte // reusable results buffer
	startTime   int64
	execs       int
	outputC     chan []byte
	downC       chan bool
	down        bool
	fnidx       uint8
}

// TestBinary handles communication with and restring of testee subprocesses.
type TestBinary struct {
	fileName      string
	commFile      string
	comm          *Mapping
	periodicCheck func()

	coverRegion []byte
	inputRegion []byte
	sonarRegion []byte

	testee       *Testee
	testeeBuffer []byte // reusable buffer for collecting testee output

	stats *Stats

	fnidx uint8
}

func init() {
	if unsafe.Offsetof(Testee{}.startTime)%8 != 0 {
		println(unsafe.Offsetof(Testee{}.startTime))
		panic("bad atomic field offset")
	}
}

// testeeBufferSize is how much output a test binary can emit
// before we start to overwrite old output.
const testeeBufferSize = 1 << 20

func newTestBinary(fileName string, periodicCheck func(), stats *Stats, fnidx uint8) *TestBinary {
	comm, err := ioutil.TempFile("", "go-fuzz-comm")
	if err != nil {
		log.Fatalf("failed to create comm file: %v", err)
	}
	comm.Truncate(CoverSize + MaxInputSize + SonarRegionSize)
	comm.Close()
	mapping, mem := createMapping(comm.Name(), CoverSize+MaxInputSize+SonarRegionSize)
	return &TestBinary{
		fileName:      fileName,
		commFile:      comm.Name(),
		comm:          mapping,
		periodicCheck: periodicCheck,
		coverRegion:   mem[:CoverSize],
		inputRegion:   mem[CoverSize : CoverSize+MaxInputSize],
		sonarRegion:   mem[CoverSize+MaxInputSize:],
		stats:         stats,
		fnidx:         fnidx,
		testeeBuffer:  make([]byte, testeeBufferSize),
	}
}

func (bin *TestBinary) close() {
	if bin.testee != nil {
		bin.testee.shutdown()
		bin.testee = nil
	}
	bin.comm.destroy()
	os.Remove(bin.commFile)
}

func (bin *TestBinary) test(data []byte) (res int, ns uint64, cover, sonar, output []byte, crashed, hanged bool) {
	if len(data) > MaxInputSize {
		panic("input is too large")
	}
	for {
		// This is the only function that is executed regularly,
		// so we tie some periodic checks to it.
		bin.periodicCheck()

		bin.stats.execs++
		if bin.testee == nil {
			bin.stats.restarts++
			bin.testee = newTestee(bin.fileName, bin.comm, bin.coverRegion, bin.inputRegion, bin.sonarRegion, bin.fnidx, bin.testeeBuffer)
		}
		var retry bool
		res, ns, cover, sonar, crashed, hanged, retry = bin.testee.test(data)
		if retry {
			bin.testee.shutdown()
			bin.testee = nil
			continue
		}
		if crashed {
			output = bin.testee.shutdown()
			if hanged {
				hdr := fmt.Sprintf("program hanged (timeout %v seconds)\n\n", *flagTimeout)
				output = append([]byte(hdr), output...)
			}
			bin.testee = nil
			return
		}
		return
	}
}

func newTestee(bin string, comm *Mapping, coverRegion, inputRegion, sonarRegion []byte, fnidx uint8, buffer []byte) *Testee {
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
	cmd := exec.Command(bin)
	if *flagTestOutput {
		// For debugging of testee failures.
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
	} else {
		cmd.Stdout = wStdout
		cmd.Stderr = wStdout
	}
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, "GOTRACEBACK=1")
	setupCommMapping(cmd, comm, rOut, wIn)
	if err = cmd.Start(); err != nil {
		// This can be a transient failure like "cannot allocate memory" or "text file is busy".
		log.Printf("failed to start test binary: %v", err)
		rIn.Close()
		wIn.Close()
		rOut.Close()
		wOut.Close()
		rStdout.Close()
		wStdout.Close()
		time.Sleep(time.Second)
		goto retry
	}
	rOut.Close()
	wIn.Close()
	wStdout.Close()
	t := &Testee{
		coverRegion: coverRegion,
		inputRegion: inputRegion,
		sonarRegion: sonarRegion,
		cmd:         cmd,
		inPipe:      rIn,
		outPipe:     wOut,
		stdoutPipe:  rStdout,
		outputC:     make(chan []byte),
		downC:       make(chan bool),
		fnidx:       fnidx,
	}
	// Stdout reader goroutine.
	go func() {
		// The testee should not output unless it crashes.
		// But there are still chances that it does. If so, it can overflow
		// the stdout pipe during testing and deadlock. To prevent the
		// deadlock we periodically read out stdout.
		// This goroutine also collects crash output.
		ticker := time.NewTicker(time.Second)
		data := buffer
		filled := 0
		for {
			select {
			case <-ticker.C:
			case <-t.downC:
			}
			n, err := t.stdoutPipe.Read(data[filled:])
			if *flagV >= 3 {
				log.Printf("testee: %v\n", string(data[filled:filled+n]))
			}
			filled += n
			if filled > testeeBufferSize/4*3 {
				copy(data, data[testeeBufferSize/2:filled])
				filled -= testeeBufferSize / 2
			}
			if err != nil {
				break
			}
		}
		ticker.Stop()
		trimmed := make([]byte, filled)
		copy(trimmed, data)
		t.outputC <- trimmed
	}()
	// Hang watcher goroutine.
	go func() {
		timeout := time.Duration(*flagTimeout) * time.Second
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
func (t *Testee) test(data []byte) (res int, ns uint64, cover, sonar []byte, crashed, hanged, retry bool) {
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

	copy(t.inputRegion[:], data)
	atomic.StoreInt64(&t.startTime, time.Now().UnixNano())
	t.writebuf[0] = t.fnidx
	binary.LittleEndian.PutUint64(t.writebuf[1:], uint64(len(data)))
	if _, err := t.outPipe.Write(t.writebuf[:]); err != nil {
		if *flagV >= 1 {
			log.Printf("write to testee failed: %v", err)
		}
		retry = true
		return
	}
	// Once we do the write, the test is running.
	// Once we read the reply below, the test is done.
	type Reply struct {
		Res   uint64
		Ns    uint64
		Sonar uint64
	}
	_, err := io.ReadFull(t.inPipe, t.resbuf[:])
	r := Reply{
		Res:   binary.LittleEndian.Uint64(t.resbuf[:]),
		Ns:    binary.LittleEndian.Uint64(t.resbuf[8:]),
		Sonar: binary.LittleEndian.Uint64(t.resbuf[16:]),
	}
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
	sonar = t.sonarRegion[:r.Sonar]
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
	if err := t.cmd.Wait(); err != nil {
		out = append(out, err.Error()...)
	}
	t.inPipe.Close()
	t.outPipe.Close()
	t.stdoutPipe.Close()
	return out
}
