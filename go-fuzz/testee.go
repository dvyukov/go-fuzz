package main

import (
	"encoding/binary"
	"log"
	"os"
	"os/exec"
	"time"
)

type Testee struct {
	coverRegion []byte
	inputRegion []byte
	cmd         *exec.Cmd
	inPipe      *os.File
	outPipe     *os.File
	stdoutPipe  *os.File
	outputC     chan []byte
	downC       chan bool
	down        bool
}

func newTestee(bin, commFile string, coverRegion, inputRegion []byte) *Testee {
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
		log.Fatalf("failed to start test binary: %v", err)
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
	go func() {
		// The testee should not output unless it crashes.
		// But there are still chances that it does. If so, it can overflow
		// the stdout pipe during testing and deadlock. To prevent the
		// deadlock we periodically read out stdout.
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
			if *flagV {
				log.Printf("testee: %v\n", string(data[filled:filled+n]))
			}
			filled += n
			if filled > N/4*3 {
				copy(data, data[N/2:filled])
				filled -= N / 2
			}
		}
		trimmed := make([]byte, filled)
		copy(trimmed, data)
		t.outputC <- trimmed
	}()
	return t
}

func (t *Testee) test(data []byte) (res int, ns int64, cover []byte, crashed, retry bool) {
	if t.down {
		log.Fatalf("cannot test: testee is already shutdown")
	}
	for i := range t.coverRegion {
		t.coverRegion[i] = 0
	}
	copy(t.inputRegion[:], data)
	if err := binary.Write(t.outPipe, binary.LittleEndian, uint64(len(data))); err != nil {
		log.Printf("write to testee failed: %v", err)
		crashed = false
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
	if err := binary.Read(t.inPipe, binary.LittleEndian, &r); err != nil {
		// Should have been crashed.
		crashed = true
		retry = false
		return
	}
	res = int(r.Res)
	ns = int64(r.Ns)
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
