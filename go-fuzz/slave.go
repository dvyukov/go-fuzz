package fuzz

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/rpc"
	"runtime/debug"
	"sort"
	"syscall"
	"testing"
	"time"
	"unsafe"
	"os/exec"
)

type fuzzer struct {
	id     int
	f      func([]byte)
	driver *rpc.Client

	corpus       map[string][]byte
	coverRegion []byte
	inputRegion []byte
	commFilename string
	lastPing     time.Time
	execs        uint64

	cmd *exec.Cmd
	inPipe *os.File
	outPipe *os.File
}

func slave() {
	rand.Seed(time.Now().UnixNano())  //!!! replace with local rand
	c, err := rpc.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("failed to dial: %v", err)
	}
	f := &fuzzer{id: id, f: ff, driver: c}
	f.main()
}

func (f *fuzzer) main() {
	ff, err := ioutil.TempFile("", "fuzz.worker") //!!! move all temp files to workdir
	if err != nil {
		log.Fatalf("failed to create rescue file: %v", err)
	}
	ff.Truncate(64<<10 + 1<<20)
	ff.Close()
	f.commFilename = ff.Name()
	fff, err := syscall.Open(ff.Name(), syscall.O_RDWR, 0)
	if err != nil {
		log.Fatalf("failed to open rescue file: %v", err)
	}
	mem, err = syscall.Mmap(fff, 0, 64<<10 + 1<<20, syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		log.Fatalf("failed to mmap rescue file: %v", err)
	}
	f.coverRegion = mem[:64<<10]
	f.inputRegion = mem[64<<10:]

	var res InitRes
	err = f.driver.Call("Driver.Init", &InitArgs{}, &res)
	if err != nil {
		//!!! handle
	}
	f.id = res.Id

	f.corpus = make(map[string][]byte)
	go func() {
		for range time.NewTicker(10 * time.Second).C {
			debug.FreeOSMemory()
		}
	}()

	for _, data := range res.Corpus {
		f.corpus[data] = []byte(data)
	}
	f.run()
}

type minLenString [][]byte

func (a minLenString) Len() int {
	return len(a)
}
func (a minLenString) Less(i, j int) bool {
	return len(a[i]) < len(a[j])
}
func (a minLenString) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (f *fuzzer) run() {
	var corpus minLenString
	for _, data := range f.corpus {
		corpus = append(corpus, data)
	}
	f.corpus = make(map[string][]byte)
	sort.Sort(minLenString(corpus))
	f.execs = 1e6
	for _, data := range corpus {
		f.exec(data)
	}
	f.execs = 0
	fmt.Printf("starting fuzzing\n")
	for {
		for _, data0 := range f.corpus {
			data := f.mutate(data0)
			f.exec(data)
			for _, data1 := range f.corpus {
				data := f.crossover(data0, data1)
				f.exec(data)
			}
		}
	}
}

func (f *fuzzer) exec(data []byte) {
	if time.Since(f.lastPing) > time.Second {
		f.lastPing = time.Now()
		res := new(PingRes)
		args := &PingArgs{Id: f.id, CorpusSize: len(f.corpus), Execs: f.execs, Coverage: testing.Coverage()}
		if err := f.driver.Call("Driver.Ping", args, res); err != nil {
			//!!! handle
		}
		for _, data1 := range res.Inputs {
			//!!! do something better than this recursion
			f.exec([]byte(data1))
		}
	}

	f.execs++

	if f.cmd == nil {
		rIn, wIn, err := os.Pipe()
		if err != nil {
			log.Fatalf("failed to pipe: %v", err)
		}
		rOut, wOut, err := os.Pipe()
		if err != nil {
			log.Fatalf("failed to pipe: %v", err)
		}
		comm, err := os.Open(commFilename)
		if err != nil {
			log.Fatalf("failed to open comm file: %v", err)
		}

		f.cmd = exec.Command(*flagBin)
		f.cmd.Stdout = os.Stdout
		f.cmd.Stderr = os.Stderr
		f.cmd.ExtraFiles = append(f.cmd.ExtraFiles, comm)
		f.cmd.ExtraFiles = append(f.cmd.ExtraFiles, rOut)
		f.cmd.ExtraFiles = append(f.cmd.ExtraFiles, wIn)
		if err = f.Start(); err != nil {
			log.Fatalf("failed to start test binary: %v", err)
		}
		comm.Close()
		rOut.Close()
		wIn.Close()
		f.inPipe = rIn
		f.outPipe = wOut
	}

	/*
	defer func() {
		err := recover()
		if err == nil {
			return
		}
		errStr := ""
		switch e := err.(type) {
		case error:
			errStr = e.Error()
		case string:
			errStr = e
		}
		args := &NewBugArgs{Id: f.id, Data: string(data), Error: errStr}
		if err := f.driver.Call("Driver.NewBug", args, nil); err != nil {
			//!!! handle
		}
	}()
	*/

	copy(f.inputRegion[:], data)
	if err := binary.Write(f.outPipe, binary.LittleEndian, len(data)); err != nil {
		//!!! handle
	}
	var res uint64
	if err := binary.Read(f.InPipe, binary.LittleEndian, &res); err != nil {
		//!!! handle
	}


	fmt.Printf("+%.04f%% on [%v]%q\n", cd*100, len(data), print)
	f.corpus[string(data)] = data

	if err := f.driver.Call("Driver.NewInput", &NewInputArgs{f.id, string(data)}, new(int)); err != nil {
		//!!! handle
	}
}

func (f *fuzzer) mutate(data []byte) []byte {
	res := make([]byte, len(data))
	copy(res, data)
	for i := f.rand(5); i >= 0; i-- {
		switch f.rand(4) {
		case 0:
			if len(res) > 0 {
				pos := f.rand(len(res))
				copy(res[pos:], res[pos+1:])
				res = res[:len(res)-1]
			}
		case 1:
			if len(res) < 100 {
				if len(res) == 0 {
					res = append(res, byte(f.rand(256)))
				} else {
					pos := f.rand(len(res))
					res = append(res, 0)
					copy(res[pos+1:], res[pos:])
					res[pos] = byte(f.rand(256))
				}
			}
		case 2:
			if len(res) > 0 {
				pos := f.rand(len(res))
				res[pos] ^= 1 << uint(f.rand(8))
			}
		case 3:
			if len(res) > 32 {
				pos0 := f.rand(len(res) - 1)
				pos1 := pos0 + f.rand(len(res)-pos0)
				copy(res[pos0:], res[pos1:])
				res = res[:len(res)-(pos1-pos0)]
			}
		}
	}
	return res
}

func (f *fuzzer) crossover(data0, data1 []byte) []byte {
	res := make([]byte, 0, len(data0)+len(data1))
	copy(res, data0)
	for i := f.rand(3); i >= 0; i-- {
		if len(data0) > 0 {
			pos0 := f.rand(len(data0))
			res = append(res, data0[:pos0]...)
			data0 = data0[pos0:]
		}
		if len(data1) > 0 {
			pos1 := f.rand(len(data1))
			res = append(res, data1[:pos1]...)
			data1 = data1[pos1:]
		}
	}
	res = append(res, data0...)
	return f.mutate(res)
}

func (f *fuzzer) rand(n int) int {
	return rand.Intn(n)
}
