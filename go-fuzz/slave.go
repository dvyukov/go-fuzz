package main

import (
	"bytes"
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
loop:
	for atomic.LoadUint32(&shutdown) == 0 {
		if len(s.newCrashers) > 0 {
			n := len(s.newCrashers) - 1
			crash := s.newCrashers[n]
			s.newCrashers[n] = NewCrasherArgs{}
			s.newCrashers = s.newCrashers[:n]
			s.handleNewCrasher(crash)
			continue loop
		}

		if len(s.newInputs) > 0 {
			n := len(s.newInputs) - 1
			input := s.newInputs[n]
			s.newInputs[n] = MasterInput{}
			s.newInputs = s.newInputs[:n]
			if err := s.master.Call("Master.NewInput", NewInputArgs{input}, nil); err != nil {
				log.Printf("new input call failed: %v", err)
			}
			continue loop
		}

		if len(s.triageQueue) > 0 {
			n := len(s.triageQueue) - 1
			input := s.triageQueue[n]
			s.triageQueue[n] = MasterInput{}
			s.triageQueue = s.triageQueue[:n]
			s.handleNewInput(input, true)
			continue loop
		}

		if len(s.inputQueue) > 0 {
			n := len(s.inputQueue) - 1
			input := s.inputQueue[n]
			s.inputQueue[n] = MasterInput{}
			s.inputQueue = s.inputQueue[:n]
			s.handleNewInput(input, false)
			continue loop
		}

		if len(s.smashQueue) > 0 {
			n := len(s.smashQueue) - 1
			input := s.smashQueue[n]
			s.smashQueue[n] = MasterInput{}
			s.smashQueue = s.smashQueue[:n]
			s.smash(input)
			continue loop
		}

		// TODO: recalculate periodically to reset freshness boost.
		if s.lastScoreLen != len(s.corpus) {
			s.recalculateScores()
			s.lastScoreLen = len(s.corpus)
		}

		data, depth := s.mutator.generate(s.corpus)
		s.testInput(data, depth)
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
		res, ns, cover, output, crashed := s.exec(inp.data)
		if crashed {
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{inp.data, output})
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
		inp.data = s.minimizeInput(inp.data, inp.cover, inp.res)
		if input.Prio < uint64(inp.res) {
			input.Prio = uint64(inp.res)
		}
		s.newInputs = append(s.newInputs, input)
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
}

func (s *Slave) handleNewCrasher(crash NewCrasherArgs) {
	crash.Data = s.minimizeCrasher(crash.Data, crash.Error)
	if err := s.master.Call("Master.NewCrasher", crash, nil); err != nil {
		log.Printf("new crasher call failed: %v", err)
	}
}

func (s *Slave) minimizeCrasher(data, error []byte) []byte {
	error = extractSuppression(error)
	tmp := s.minimize(data, func(candidate, cover, output []byte, res int, crashed bool) bool {
		if !crashed {
			return false
		}
		if !bytes.Equal(error, extractSuppression(output)) {
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{candidate, output})
			return false
		}
		return true
	})
	if *flagV >= 1 {
		log.Printf("minimized crasher [%v]%q -> [%v]%q", len(data), data, len(tmp), tmp)
	}
	return tmp
}

func (s *Slave) minimizeInput(data, cover []byte, res int) []byte {
	tmp := s.minimize(data, func(candidate, cover1, output []byte, res1 int, crashed bool) bool {
		if crashed {
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{candidate, output})
			return false
		}
		if res != res1 {
			return false
		}
		if !bytes.Equal(cover, cover1) {
			// TODO: this can be a new intersting input.
			return false
		}
		return true
	})
	if *flagV >= 1 {
		log.Printf("minimized input [%v]%q -> [%v]%q", len(data), data, len(tmp), tmp)
	}
	return tmp
}

func (s *Slave) minimize(data []byte, pred func(candidate, cover, output []byte, result int, crashed bool) bool) []byte {
	res := make([]byte, len(data))
	copy(res, data)

	// First, try to cut tail.
	for n := 1024; n != 0; n /= 2 {
		for len(res) > n {
			candidate := res[:len(res)-n]
			result, _, cover, output, crashed := s.exec(candidate)
			if !pred(candidate, cover, output, result, crashed) {
				break
			}
			res = candidate
		}
	}

	// Then, try to remove each individual byte.
	for i := 0; i < len(res); i++ {
		candidate := make([]byte, len(res)-1)
		copy(candidate[:i], res[:i])
		copy(candidate[i:], res[i+1:])
		result, _, cover, output, crashed := s.exec(candidate)
		if !pred(candidate, cover, output, result, crashed) {
			continue
		}
		res = candidate
		i--
	}
	return res
}

func (s *Slave) smash(input MasterInput) {
	data := input.Data
	depth := int(input.Prio)

	// TODO: auto-detect magic bytes and strings in inputs and insert them more frequently.

	_, _ = data, depth
	/*
			// Stage 0: flip each bit one-by-one.
			for i := 0; i < len(data)*8; i++ {
				data[i/8] ^= 1 << uint(i%8)
				s.testInput(data, depth)
				data[i/8] ^= 1 << uint(i%8)
			}

			// Stage 1: two walking bits.
			for i := 0; i < len(data)*8-1; i++ {
				data[i/8] ^= 1 << uint(i%8)
				data[(i+1)/8] ^= 1 << uint((i+1)%8)
				s.testInput(data, depth)
				data[i/8] ^= 1 << uint(i%8)
				data[(i+1)/8] ^= 1 << uint((i+1)%8)
			}

			// Stage 2: four walking bits.
			for i := 0; i < len(data)*8-3; i++ {
				data[i/8] ^= 1 << uint(i%8)
				data[(i+1)/8] ^= 1 << uint((i+1)%8)
				data[(i+2)/8] ^= 1 << uint((i+2)%8)
				data[(i+3)/8] ^= 1 << uint((i+3)%8)
				s.testInput(data, depth)
				data[i/8] ^= 1 << uint(i%8)
				data[(i+1)/8] ^= 1 << uint((i+1)%8)
				data[(i+2)/8] ^= 1 << uint((i+2)%8)
				data[(i+3)/8] ^= 1 << uint((i+3)%8)
			}

		// Stage 3: byte flip.
		for i := 0; i < len(data); i++ {
			data[i] ^= 0xff
			s.testInput(data, depth)
			data[i] ^= 0xff
		}

			// Stage 4: two walking bytes.
			for i := 0; i < len(data)-1; i++ {
				data[i] ^= 0xff
				data[i+1] ^= 0xff
				s.testInput(data, depth)
				data[i] ^= 0xff
				data[i+1] ^= 0xff
			}

			// Stage 5: four walking bytes.
			for i := 0; i < len(data)-3; i++ {
				data[i] ^= 0xff
				data[i+1] ^= 0xff
				data[i+2] ^= 0xff
				data[i+3] ^= 0xff
				s.testInput(data, depth)
				data[i] ^= 0xff
				data[i+1] ^= 0xff
				data[i+2] ^= 0xff
				data[i+3] ^= 0xff
			}
	*/

	var res DoneSmashingRes
	if err := s.master.Call("Master.DoneSmashing", DoneSmashingArgs{s.id, input}, &res); err != nil {
		log.Printf("done smashing call failed: %v", err)
	}
	if res.Smash.Data != nil {
		s.smashQueue = append(s.smashQueue, res.Smash)
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
		if inp.depth < 10 {
			// no boost for you
		} else if inp.depth < 20 {
			score *= 2
		} else if inp.depth < 40 {
			score *= 3
		} else if inp.depth < 80 {
			score *= 4
		} else {
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

func (s *Slave) testInput(data []byte, depth int) {
	if len(s.hangingInputs) > 0 {
		if _, ok := s.hangingInputs[hash(data)]; ok {
			return // no, thanks
		}
	}
	_, _, cover, output, crashed := s.exec(data)
	if crashed {
		s.newCrashers = append(s.newCrashers, NewCrasherArgs{data, output})
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

func (s *Slave) exec(data []byte) (res int, ns uint64, cover, output []byte, crashed bool) {
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
			output = s.testee.shutdown()
			s.testee = nil
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
	for i, x := range cur {
		// Quantize the counters.
		// Otherwise we get too inflated corpus.
		if x == 0 {
			x = 0
		} else if x <= 1 {
			x = 1
		} else if x <= 2 {
			x = 2
		} else if x <= 4 {
			x = 4
		} else if x <= 8 {
			x = 8
		} else if x <= 16 {
			x = 16
		} else if x <= 32 {
			x = 32
		} else if x <= 64 {
			x = 64
		} else {
			x = 255
		}
		v := base[i]
		if v < x {
			base[i] = x
		}
	}
}
