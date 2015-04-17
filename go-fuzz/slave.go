package main

import (
	"io/ioutil"
	"log"
	"net/rpc"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	coverSize    = 64 << 10
	maxInputSize = 1 << 20
	syncPeriod   = 10 * time.Second
	syncDeadline = 5 * syncPeriod
)

type Slave struct {
	id      int
	f       func([]byte)
	master  *rpc.Client
	mutator *Mutator

	maxCover      []byte
	corpus        []Input
	corpusSigs    map[Sig]struct{}
	triageQueue   []MasterInput
	inputQueue    []MasterInput
	smashInputs   []MasterInput
	newInputs     []MasterInput
	newBugs       []NewBugArgs
	hangingInputs map[Sig]struct{}

	coverRegion []byte
	inputRegion []byte
	commFile    string
	lastSync    time.Time

	statExecs    uint64
	statRestarts uint64

	testee *Testee
}

type Input struct {
	data     []byte
	cover    []byte
	res      int
	depth    int
	execTime int64
}

func slaveMain() {
	c, err := rpc.Dial("tcp", *flagSlave)
	if err != nil {
		log.Fatalf("failed to dial: %v", err)
	}
	s := &Slave{master: c}
	s.setupCommFile()
	s.mutator = newMutator()
	s.maxCover = make([]byte, coverSize)
	s.corpusSigs = make(map[Sig]struct{})
	s.hangingInputs = make(map[Sig]struct{})

	var res ConnectRes
	err = s.master.Call("Master.Connect", &ConnectArgs{}, &res)
	if err != nil {
		log.Fatalf("failed to connect to master: %v", err)
	}
	s.id = res.ID
	s.triageQueue = res.Bootstrap
	s.inputQueue = res.Corpus

	s.loop()
}

func (s *Slave) setupCommFile() {
	comm, err := ioutil.TempFile("", "go-fuzz-comm")
	if err != nil {
		log.Fatalf("failed to create comm file: %v", err)
	}
	comm.Truncate(coverSize + maxInputSize)
	comm.Close()
	s.commFile = comm.Name()
	fd, err := syscall.Open(comm.Name(), syscall.O_RDWR, 0)
	if err != nil {
		log.Fatalf("failed to open comm file: %v", err)
	}
	mem, err := syscall.Mmap(fd, 0, coverSize+maxInputSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		log.Fatalf("failed to mmap comm file: %v", err)
	}
	s.coverRegion = mem[:coverSize]
	s.inputRegion = mem[coverSize:]
}

func (s *Slave) loop() {
	for atomic.LoadUint32(&shutdown) == 0 {
		for len(s.newBugs) > 0 {
			n := len(s.newBugs) - 1
			bug := s.newBugs[n]
			s.newBugs[n] = NewBugArgs{}
			s.newBugs = s.newBugs[:n]
			if err := s.master.Call("Master.NewBug", bug, nil); err != nil {
				log.Printf("new bug call failed: %v", err)
			}
		}

		for len(s.newInputs) > 0 {
			n := len(s.newInputs) - 1
			input := s.newInputs[n]
			s.newInputs[n] = MasterInput{}
			s.newInputs = s.newInputs[:n]
			if err := s.master.Call("Master.NewInput", NewInputArgs{input}, nil); err != nil {
				log.Printf("new input call failed: %v", err)
			}
		}

		for len(s.triageQueue) > 0 {
			n := len(s.triageQueue) - 1
			input := s.triageQueue[n]
			s.triageQueue[n] = MasterInput{}
			s.triageQueue = s.triageQueue[:n]
			s.handleNewInput(input, true)
		}

		for len(s.inputQueue) > 0 {
			n := len(s.inputQueue) - 1
			input := s.inputQueue[n]
			s.inputQueue[n] = MasterInput{}
			s.inputQueue = s.inputQueue[:n]
			s.handleNewInput(input, false)
		}

		data, depth := s.mutator.generate(s.corpus)
		s.exec(data, depth)
	}
}

func (s *Slave) handleNewInput(input MasterInput, triage bool) {
	sig := hash(input.Data)
	if _, ok := s.corpusSigs[sig]; ok {
		return // already have this
	}
	if _, ok := s.hangingInputs[sig]; ok {
		return // no, thanks
	}
	inp := Input{data: input.Data, depth: int(input.Prio), execTime: 1 << 60}
	for i := 0; i < 3; i++ {
		res, ns, cover, crashed := s.execImpl(inp.data, inp.depth)
		if crashed {
			return // inputs in corpus should not crash
		}
		if inp.cover == nil {
			inp.cover = make([]byte, coverSize)
			copy(inp.cover, cover)
		} else {
			for i, v := range cover {
				x := inp.cover[i]
				if v < x {
					inp.cover[i] = v
				}
			}
		}
		if inp.res < res {
			inp.res = res
		}
		if inp.execTime > ns {
			inp.execTime = ns
		}
	}
	if triage {
		// TODO: minimize input
	}
	updateCover(s.maxCover, inp.cover)
	s.corpusSigs[sig] = struct{}{}
	s.corpus = append(s.corpus, inp)
	if triage {
		s.newInputs = append(s.newInputs, input)
	}
}

func (s *Slave) exec(data []byte, depth int) {
	if _, ok := s.hangingInputs[hash(data)]; ok {
		return // no, thanks
	}
	_, _, cover, crashed := s.execImpl(data, depth)
	if crashed {
		return
	}
	newCover, newCount := compareCover(s.maxCover, cover)
	if !newCover && !newCount {
		return
	}
	updateCover(s.maxCover, cover)
	input := MasterInput{data, uint64(depth)}
	s.triageQueue = append(s.triageQueue, input)
	/*
		print := input.Data
		if len(print) > 50 {
			print = print[:50]
		}
		fmt.Printf("new cover(%v)/count(%v) on [%v]%q\n", newCover, newCount, len(data), print)
	*/
}

func (s *Slave) execImpl(data []byte, depth int) (res int, ns int64, cover []byte, crashed bool) {
	for {
		if atomic.LoadUint32(&shutdown) != 0 {
			select {}
		}
		s.sendSync()

		s.statExecs++
		if s.testee == nil {
			s.statRestarts++
			s.testee = newTestee(*flagBin, s.commFile, s.coverRegion, s.inputRegion)
		}
		var hang, retry bool
		res, ns, cover, crashed, hang, retry = s.testee.test(data)
		if retry {
			s.testee.shutdown()
			s.testee = nil
			continue
		}
		if crashed || hang {
			if hang {
				s.hangingInputs[hash(data)] = struct{}{}
				crashed = true
			}
			out := s.testee.shutdown()
			s.testee = nil
			s.newBugs = append(s.newBugs, NewBugArgs{data, out})
			return
		}
		return
	}
}

func (s *Slave) sendSync() {
	if time.Since(s.lastSync) < syncPeriod {
		return
	}
	s.lastSync = time.Now()
	res := new(SyncRes)
	args := &SyncArgs{ID: s.id, Execs: s.statExecs, Restarts: s.statRestarts}
	s.statExecs = 0
	s.statRestarts = 0
	if err := s.master.Call("Master.Sync", args, res); err != nil {
		log.Printf("sync call failed: %v", err)
		return
	}
	s.inputQueue = append(s.inputQueue, res.Inputs...)
}

func compareCover(base, cur []byte) (bool, bool) {
	if len(base) != coverSize || len(cur) != coverSize {
		log.Fatalf("bad cover table size (%v, %v)", len(base), len(cur))
	}
	cnt := false
	for i, v := range base {
		x := cur[i]
		if v == 0 && x != 0 {
			return true, true
		}
		if x > v {
			cnt = true
		}
	}
	return false, cnt
}

func updateCover(base, cur []byte) {
	if len(base) != coverSize || len(cur) != coverSize {
		log.Fatalf("bad cover table size (%v, %v)", len(base), len(cur))
	}
	for i, v := range cur {
		x := base[i]
		if x < v {
			base[i] = v
		}
	}
}
