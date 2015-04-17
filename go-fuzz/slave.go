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
	syncPeriod   = 3 * time.Second
	syncDeadline = 10 * syncPeriod
)

type Slave struct {
	id      int
	f       func([]byte)
	master  *rpc.Client
	mutator *Mutator

	startTime     int64
	maxCover      []byte
	corpus        []Input
	corpusSigs    map[Sig]struct{}
	triageQueue   []MasterInput
	inputQueue    []MasterInput
	smashQueue    []MasterInput
	newInputs     []MasterInput
	newCrashers   []NewCrasherArgs
	hangingInputs map[Sig]struct{}

	coverRegion []byte
	inputRegion []byte
	commFile    string
	lastSync    time.Time

	statExecs    uint64
	statRestarts uint64

	testee *Testee

	lastScoreLen int
}

type Input struct {
	data            []byte
	cover           []byte
	coverSize       int
	res             int
	depth           int
	execTime        uint64
	boost           int
	addTime         int64
	score           int
	runningScoreSum int
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
	s.startTime = time.Now().UnixNano()

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
		for len(s.newCrashers) > 0 {
			n := len(s.newCrashers) - 1
			bug := s.newCrashers[n]
			s.newCrashers[n] = NewCrasherArgs{}
			s.newCrashers = s.newCrashers[:n]
			if err := s.master.Call("Master.NewCrasher", bug, nil); err != nil {
				log.Printf("new crasher call failed: %v", err)
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

		for len(s.smashQueue) > 0 {
			n := len(s.smashQueue) - 1
			input := s.smashQueue[n]
			s.smashQueue[n] = MasterInput{}
			s.smashQueue = s.smashQueue[:n]
			s.smash(input)
			if err := s.master.Call("Master.DoneSmashing", DoneSmashingArgs{s.id, input}, nil); err != nil {
				log.Printf("done smashing call failed: %v", err)
			}
		}

		if s.lastScoreLen != len(s.corpus) {
			s.recalculateScores()
			s.lastScoreLen = len(s.corpus)
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
	inp := Input{data: input.Data, depth: int(input.Prio), execTime: 1 << 60, addTime: time.Now().UnixNano()}
	// Calculate min exec time, min coverage and max result of 3 runs.
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
	inp.coverSize = 0
	for _, v := range inp.cover {
		if v != 0 {
			inp.coverSize++
		}
	}
	s.corpusSigs[sig] = struct{}{}
	s.corpus = append(s.corpus, inp)
	if triage {
		if input.Prio < uint64(inp.res) {
			input.Prio = uint64(inp.res)
		}
		s.newInputs = append(s.newInputs, input)
	}
}

func (s *Slave) smash(input MasterInput) {
	data := input.Data
	depth := int(input.Prio)

	// Stage 0: flip each bit one-by-one.
	for i := 0; i < len(data)*8; i++ {
		data[i/8] ^= 1 << uint(i%8)
		s.exec(data, depth)
		data[i/8] ^= 1 << uint(i%8)
	}

	// Stage 1: two walking bits.
	for i := 0; i < len(data)*8-1; i++ {
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
		s.exec(data, depth)
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
	}

	// Stage 2: four walking bits.
	for i := 0; i < len(data)*8-3; i++ {
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
		data[(i+2)/8] ^= 1 << uint((i+2)%8)
		data[(i+3)/8] ^= 1 << uint((i+3)%8)
		s.exec(data, depth)
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
		data[(i+2)/8] ^= 1 << uint((i+2)%8)
		data[(i+3)/8] ^= 1 << uint((i+3)%8)
	}

	// Stage 3: byte flip.
	for i := 0; i < len(data); i++ {
		data[i] ^= 0xff
		s.exec(data, depth)
		data[i] ^= 0xff
	}

	// Stage 4: two walking bytes.
	for i := 0; i < len(data)-1; i++ {
		data[i] ^= 0xff
		data[i+1] ^= 0xff
		s.exec(data, depth)
		data[i] ^= 0xff
		data[i+1] ^= 0xff
	}

	// Stage 5: four walking bytes.
	for i := 0; i < len(data)-3; i++ {
		data[i] ^= 0xff
		data[i+1] ^= 0xff
		data[i+2] ^= 0xff
		data[i+3] ^= 0xff
		s.exec(data, depth)
		data[i] ^= 0xff
		data[i+1] ^= 0xff
		data[i+2] ^= 0xff
		data[i+3] ^= 0xff
	}

}

func (s *Slave) recalculateScores() {
	var sumExecTime, sumCoverSize uint64
	for _, inp := range s.corpus {
		sumExecTime += inp.execTime
		sumCoverSize += uint64(inp.coverSize)
	}
	n := uint64(len(s.corpus))
	avgExecTime := sumExecTime / n
	avgCoverSize := sumCoverSize / n

	scoreSum := 0
	now := int64(time.Now().UnixNano())
	for i, inp := range s.corpus {
		score := 10.0

		// Execution time multiplier 0.1-3x.
		// Fuzzing faster inputs increases efficiency.
		execTime := float64(inp.execTime) / float64(avgExecTime)
		if execTime > 10 {
			score /= 10
		} else if execTime > 4 {
			score /= 4
		} else if execTime > 4 {
			score /= 4
		} else if execTime > 2 {
			score /= 2
		} else if execTime < 0.25 {
			score *= 3
		} else if execTime < 0.33 {
			score *= 2
		} else if execTime < 0.5 {
			score *= 1.5
		}

		// Coverage size multiplier 0.25-3x.
		// Inputs with larger coverage are more interesting.
		coverSize := float64(inp.coverSize) / float64(avgCoverSize)
		if coverSize > 3 {
			score *= 3
		} else if coverSize > 2 {
			score *= 2
		} else if coverSize > 1.5 {
			score *= 1.5
		} else if coverSize < 0.3 {
			score /= 4
		} else if coverSize < 0.5 {
			score /= 2
		} else if coverSize < 0.75 {
			score /= 1.5
		}

		// Input depth multiplier 1-5x.
		// Deeper inputs have higher chances of digging deeper into code.
		switch inp.depth {
		case 0, 1, 2, 3:
			// no boost for you
		case 4, 5, 6, 7:
			score *= 2
		case 8, 9, 10, 11:
			score *= 3
		case 12, 13, 14, 15:
			score *= 4
		default:
			score *= 5
		}

		// User boost (Fuzz function return value) multiplier 1-4x.
		// We don't know what it is, but user said so.
		switch inp.res {
		case 0:
			// no boost for you
		case 1:
			// Assuming this is a correct input (e.g. deserialized successfully).
			score *= 2
		default:
			// Assuming this is a correct and interesting in some way input.
			score *= 4
		}

		// New inputs get 3x boost for the first hour to catch up with the rest.
		if now-s.startTime > int64(time.Hour) && now-inp.addTime < int64(time.Hour) {
			score *= 3
		}

		if score < 1 {
			score = 1
		}
		if score > 1000 {
			score = 1000
		}
		scoreSum += int(score)
		s.corpus[i].score = int(score)
		s.corpus[i].runningScoreSum = scoreSum
	}
}

func (s *Slave) exec(data []byte, depth int) {
	if len(s.hangingInputs) > 0 {
		if _, ok := s.hangingInputs[hash(data)]; ok {
			return // no, thanks
		}
	}
	_, _, cover, crashed := s.execImpl(data, depth)
	if crashed {
		return
	}
	newCover, newCount := compareCover(s.maxCover, cover)
	if !newCover && !newCount {
		return
	}
	// TODO: give more priority for newCover
	updateCover(s.maxCover, cover)
	input := MasterInput{data, uint64(depth)}
	s.triageQueue = append(s.triageQueue, input)
}

func (s *Slave) execImpl(data []byte, depth int) (res int, ns uint64, cover []byte, crashed bool) {
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
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{data, out})
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
	if res.Smash.Data != nil {
		s.smashQueue = append(s.smashQueue, res.Smash)
	}
}

func compareCover(base, cur []byte) (bool, bool) {
	if len(base) != coverSize || len(cur) != coverSize {
		log.Fatalf("bad cover table size (%v, %v)", len(base), len(cur))
	}
	newCover, newCounter := compareCoverBody(&base[0], &cur[0])
	if false {
		newCover1, newCounter1 := compareCoverDump(base, cur)
		if newCover != newCover1 || newCounter != newCounter1 {
			panic("bad")
		}
	}
	return newCover, newCounter
}

func compareCoverDump(base, cur []byte) (bool, bool) {
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

func compareCoverBody(base, cur *byte) (bool, bool) // in compare.s

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
