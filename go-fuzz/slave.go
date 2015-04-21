package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/rpc"
	"sync"
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

// Hub contains data shared between all slaves in the process (e.g. corpus).
// This reduces memory consumption for highly parallel slaves.
type Hub struct {
	id            int
	procs         int
	master        *rpc.Client
	mu            sync.RWMutex
	lastSync      time.Time
	maxCover      []byte
	maxCoverSize  int
	corpus        []Input
	corpusSigs    map[Sig]struct{}
	triageQueue   []MasterInput
	inputQueue    []MasterInput
	smashQueue    []MasterInput
	newInputs     []MasterInput
	newCrashers   []NewCrasherArgs
	hangingInputs map[Sig]struct{}
	scoredLen     int

	totalExecs    uint64
	totalRestarts uint64
}

type Slave struct {
	*Hub
	mutator *Mutator

	coverRegion []byte
	inputRegion []byte
	commFile    string

	lastStatUpdate time.Time
	statExecs      uint64
	statRestarts   uint64

	testee *Testee
}

type Input struct {
	data            []byte
	cover           []byte
	coverSize       int
	res             int
	depth           int
	execTime        uint64
	boost           int
	score           int
	runningScoreSum int
}

func slaveMain(procs int) {
	c, err := rpc.Dial("tcp", *flagSlave)
	if err != nil {
		log.Fatalf("failed to dial: %v", err)
	}
	var res ConnectRes
	if err := c.Call("Master.Connect", &ConnectArgs{Procs: procs}, &res); err != nil {
		log.Fatalf("failed to connect to master: %v", err)
	}

	hub := &Hub{
		procs:         procs,
		id:            res.ID,
		master:        c,
		maxCover:      make([]byte, coverSize),
		corpusSigs:    make(map[Sig]struct{}),
		hangingInputs: make(map[Sig]struct{}),
		triageQueue:   res.Bootstrap,
		inputQueue:    res.Corpus,
	}

	for i := 0; i < procs; i++ {
		s := &Slave{
			Hub:     hub,
			mutator: newMutator(),
		}
		s.setupCommFile()
		go s.loop()
	}
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
		s.mu.RLock()
		if len(s.newCrashers) > 0 ||
			len(s.newInputs) > 0 ||
			len(s.triageQueue) > 0 ||
			len(s.inputQueue) > 0 ||
			len(s.smashQueue) > 0 ||
			s.scoredLen != len(s.corpus) {
			s.mu.RUnlock()
			s.mu.Lock()

			if len(s.newCrashers) > 0 {
				n := len(s.newCrashers) - 1
				crash := s.newCrashers[n]
				s.newCrashers[n] = NewCrasherArgs{}
				s.newCrashers = s.newCrashers[:n]
				s.mu.Unlock()
				s.mu.RLock()
				s.handleNewCrasher(crash)
				s.mu.RUnlock()
				continue
			}

			if len(s.newInputs) > 0 {
				n := len(s.newInputs) - 1
				input := s.newInputs[n]
				s.newInputs[n] = MasterInput{}
				s.newInputs = s.newInputs[:n]
				s.mu.Unlock()
				if err := s.master.Call("Master.NewInput", NewInputArgs{input}, nil); err != nil {
					log.Printf("new input call failed: %v", err)
				}
				continue
			}

			if len(s.triageQueue) > 0 {
				n := len(s.triageQueue) - 1
				input := s.triageQueue[n]
				s.triageQueue[n] = MasterInput{}
				s.triageQueue = s.triageQueue[:n]
				s.mu.Unlock()
				s.mu.RLock()
				s.handleNewInput(input, true)
				s.mu.RUnlock()
				continue
			}

			if len(s.inputQueue) > 0 {
				n := len(s.inputQueue) - 1
				input := s.inputQueue[n]
				s.inputQueue[n] = MasterInput{}
				s.inputQueue = s.inputQueue[:n]
				s.mu.Unlock()
				s.mu.RLock()
				s.handleNewInput(input, false)
				s.mu.RUnlock()
				continue
			}

			if len(s.smashQueue) > 0 {
				n := len(s.smashQueue) - 1
				input := s.smashQueue[n]
				s.smashQueue[n] = MasterInput{}
				s.smashQueue = s.smashQueue[:n]
				s.mu.Unlock()
				s.mu.RLock()
				s.smash(input)
				s.mu.RUnlock()
				continue
			}

			if s.scoredLen != len(s.corpus) {
				s.recalculateScores()
				s.scoredLen = len(s.corpus)
				s.mu.Unlock()
				continue
			}
			s.mu.Unlock()
			continue
		}

		data, depth := s.mutator.generate(s.corpus)
		s.testInput(data, depth)
		s.mu.RUnlock()
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
	// Calculate min exec time, min coverage and max result of 3 runs.
	for i := 0; i < 3; i++ {
		res, ns, cover, output, crashed := s.exec(inp.data)
		if crashed {
			// Inputs in corpus should not crash.
			s.mu.RUnlock()
			s.mu.Lock()
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{inp.data, output})
			s.mu.Unlock()
			s.mu.RLock()
			return
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
	inp.coverSize = 0
	for _, v := range inp.cover {
		if v != 0 {
			inp.coverSize++
		}
	}
	if triage {
		inp.data = s.minimizeInput(inp.data, inp.cover, inp.res)
		if input.Prio < uint64(inp.res) {
			input.Prio = uint64(inp.res)
		}
	}
	s.mu.RUnlock()
	s.mu.Lock()
	if triage {
		s.newInputs = append(s.newInputs, input)
	}
	s.updateMaxCover(inp.cover)
	s.corpusSigs[sig] = struct{}{}
	s.corpus = append(s.corpus, inp)
	s.mu.Unlock()
	s.mu.RLock()
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
			s.mu.RUnlock()
			s.mu.Lock()
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{candidate, output})
			s.mu.Unlock()
			s.mu.RLock()
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
			s.mu.RUnlock()
			s.mu.Lock()
			s.newCrashers = append(s.newCrashers, NewCrasherArgs{candidate, output})
			s.mu.Unlock()
			s.mu.RLock()
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

	// Stage 0: flip each bit one-by-one.
	for i := 0; i < len(data)*8; i++ {
		data[i/8] ^= 1 << uint(i%8)
		s.testInput(data, depth)
		data[i/8] ^= 1 << uint(i%8)
	}

	/*
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
	*/

	// Stage 3: byte flip.
	for i := 0; i < len(data); i++ {
		data[i] ^= 0xff
		s.testInput(data, depth)
		data[i] ^= 0xff
	}

	/*
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

	// arith for bytes
	// arith for shorts (both endianess)
	// arith for ints (both endianess)
	// set to interesting_8
	// set to interesting_16 (both endianess)
	// set to interesting_32 (both endianess)

	// Trim after every byte.
	for i := 1; i < len(data); i++ {
		tmp := data[:i]
		s.testInput(tmp, depth)
	}

	// Do a bunch of random mutations so that this input catches up with the rest.
	for i := 0; i < 1e4; i++ {
		tmp := s.mutator.mutate(data, s.corpus)
		s.testInput(tmp, depth+1)
	}

	var res DoneSmashingRes
	if err := s.master.Call("Master.DoneSmashing", DoneSmashingArgs{s.id, input}, &res); err != nil {
		log.Printf("done smashing call failed: %v", err)
	}
	if res.Smash.Data != nil {
		s.mu.RUnlock()
		s.mu.Lock()
		s.smashQueue = append(s.smashQueue, res.Smash)
		s.mu.Unlock()
		s.mu.RLock()
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
		s.mu.RUnlock()
		s.mu.Lock()
		s.newCrashers = append(s.newCrashers, NewCrasherArgs{data, output})
		s.mu.Unlock()
		s.mu.RLock()
		return
	}
	newCover, newCount := compareCover(s.maxCover, cover)
	if !newCover && !newCount {
		return
	}
	s.mu.RUnlock()
	s.mu.Lock()
	// TODO: give more priority for newCover
	s.updateMaxCover(cover)
	input := MasterInput{data, uint64(depth)}
	s.triageQueue = append(s.triageQueue, input)
	s.mu.Unlock()
	s.mu.RLock()
}

func (s *Slave) exec(data []byte) (res int, ns uint64, cover, output []byte, crashed bool) {
	for {
		// This is the only function that is executed regularly,
		// so we tie some periodic checks to it.
		s.periodicCheck()

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
				s.mu.RUnlock()
				s.mu.Lock()
				s.hangingInputs[hash(data)] = struct{}{}
				s.mu.Unlock()
				s.mu.RLock()
				crashed = true
			}
			output = s.testee.shutdown()
			s.testee = nil
			return
		}
		return
	}
}

func (s *Slave) periodicCheck() {
	if atomic.LoadUint32(&shutdown) != 0 {
		select {}
	}
	if time.Since(s.lastStatUpdate) < syncPeriod {
		return
	}
	s.mu.RUnlock()
	s.mu.Lock()
	s.totalExecs += s.statExecs
	s.statExecs = 0
	s.totalRestarts += s.statRestarts
	s.statRestarts = 0
	if time.Since(s.lastSync) >= syncPeriod {
		res := new(SyncRes)
		args := &SyncArgs{
			ID:            s.id,
			Execs:         s.totalExecs,
			Restarts:      s.totalRestarts,
			CoverFullness: float64(s.maxCoverSize) / coverSize,
		}
		s.totalExecs = 0
		s.totalRestarts = 0
		if err := s.master.Call("Master.Sync", args, res); err != nil {
			log.Printf("sync call failed: %v", err)
		} else {
			s.inputQueue = append(s.inputQueue, res.Inputs...)
			if res.Smash.Data != nil {
				s.smashQueue = append(s.smashQueue, res.Smash)
			}
		}
		s.lastSync = time.Now()
	}
	s.mu.Unlock()
	s.mu.RLock()
}

func (s *Slave) updateMaxCover(cur []byte) {
	base := s.maxCover
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
			if v == 0 {
				s.maxCoverSize++
			}
			base[i] = x
		}
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
