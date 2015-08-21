// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

const (
	execBootstrap = iota
	execCorpus
	execMinimizeInput
	execMinimizeCrasher
	execTriageInput
	execFuzz
	execVersifier
	execSmash
	execSonar
	execSonarHint
	execTotal
	execCount
)

// Slave manages one testee.
type Slave struct {
	id      int
	hub     *Hub
	mutator *Mutator

	coverBin *TestBinary
	sonarBin *TestBinary

	triageQueue  []MasterInput
	crasherQueue []NewCrasherArgs

	lastSync time.Time
	stats    Stats
	execs    [execCount]uint64
}

type Input struct {
	mine            bool
	data            []byte
	cover           []byte
	coverSize       int
	res             int
	depth           int
	typ             int
	execTime        uint64
	favored         bool
	score           int
	runningScoreSum int
}

func slaveMain() {
	zipr, err := zip.OpenReader(*flagBin)
	if err != nil {
		log.Fatalf("failed to open bin file: %v", err)
	}
	var coverBin, sonarBin string
	var metadata MetaData
	for _, zipf := range zipr.File {
		r, err := zipf.Open()
		if err != nil {
			log.Fatalf("failed to uzip file from input archive: %v", err)
		}
		if zipf.Name == "metadata" {
			if err := json.NewDecoder(r).Decode(&metadata); err != nil {
				log.Fatalf("failed to decode metadata: %v", err)
			}
		} else {
			f, err := ioutil.TempFile("", "go-fuzz")
			if err != nil {
				log.Fatalf("failed to create temp file: %v", err)
			}
			f.Close()
			os.Remove(f.Name())
			f, err = os.OpenFile(f.Name()+".exe", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
			if err != nil {
				log.Fatalf("failed to create temp file: %v", err)
			}
			if _, err := io.Copy(f, r); err != nil {
				log.Fatalf("failed to uzip bin file: %v", err)
			}
			f.Close()
			switch zipf.Name {
			case "cover.exe":
				coverBin = f.Name()
			case "sonar.exe":
				sonarBin = f.Name()
			default:
				log.Fatalf("unknown file '%v' in input archive", f.Name())
			}
		}
		r.Close()
	}
	zipr.Close()
	if coverBin == "" || sonarBin == "" || len(metadata.Blocks) == 0 {
		log.Fatalf("bad input archive: missing file")
	}

	shutdownCleanup = append(shutdownCleanup, func() {
		os.Remove(coverBin)
		os.Remove(sonarBin)
	})

	hub := newHub(metadata)
	for i := 0; i < *flagProcs; i++ {
		s := &Slave{
			id:      i,
			hub:     hub,
			mutator: newMutator(),
		}
		s.coverBin = newTestBinary(coverBin, s.periodicCheck, &s.stats)
		s.sonarBin = newTestBinary(sonarBin, s.periodicCheck, &s.stats)
		go s.loop()
	}
}

func (s *Slave) loop() {
	iter, fuzzSonarIter, versifierSonarIter := 0, 0, 0
	for atomic.LoadUint32(&shutdown) == 0 {
		if len(s.crasherQueue) > 0 {
			n := len(s.crasherQueue) - 1
			crash := s.crasherQueue[n]
			s.crasherQueue[n] = NewCrasherArgs{}
			s.crasherQueue = s.crasherQueue[:n]
			if *flagV >= 2 {
				log.Printf("slave %v processes crasher [%v]%v", s.id, len(crash.Data), hash(crash.Data))
			}
			s.processCrasher(crash)
			continue
		}

		select {
		case input := <-s.hub.triageC:
			if *flagV >= 2 {
				log.Printf("slave %v triages master input [%v]%v minimized=%v smashed=%v", s.id, len(input.Data), hash(input.Data), input.Minimized, input.Smashed)
			}
			s.triageInput(input)
			for {
				x := atomic.LoadUint32(&s.hub.initialTriage)
				if x == 0 || atomic.CompareAndSwapUint32(&s.hub.initialTriage, x, x-1) {
					break
				}
			}
			continue
		default:
		}

		if atomic.LoadUint32(&s.hub.initialTriage) != 0 {
			// Other slaves are still triaging initial inputs.
			// Wait until they finish, otherwise we can generate
			// as if new interesting inputs that are not actually new
			// and thus unnecessary inflate corpus on every run.
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if len(s.triageQueue) > 0 {
			n := len(s.triageQueue) - 1
			input := s.triageQueue[n]
			s.triageQueue[n] = MasterInput{}
			s.triageQueue = s.triageQueue[:n]
			if *flagV >= 2 {
				log.Printf("slave %v triages local input [%v]%v minimized=%v smashed=%v", s.id, len(input.Data), hash(input.Data), input.Minimized, input.Smashed)
			}
			s.triageInput(input)
			continue
		}

		ro := s.hub.ro.Load().(*ROData)
		if len(ro.corpus) == 0 {
			// Some other slave triages corpus inputs.
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 9 out of 10 iterations are random fuzzing.
		iter++
		if iter%10 != 0 || ro.verse == nil {
			data, depth := s.mutator.generate(ro)
			// Every 1000-th iteration goes to sonar.
			fuzzSonarIter++
			if *flagSonar && fuzzSonarIter%1000 == 0 {
				// TODO: ensure that generated hint inputs does not actually take 99% of time.
				sonar := s.testInputSonar(data, depth)
				s.processSonarData(data, sonar, depth, false)
			} else {
				// Plain old blind fuzzing.
				s.testInput(data, depth, execFuzz)
			}
		} else {
			// 1 out of 10 iterations goes to versifier.
			data := ro.verse.Rhyme()
			const maxSize = MaxInputSize - 5*SonarMaxLen // need some gap for sonar replacements
			if len(data) > maxSize {
				data = data[:maxSize]
			}
			// Every 100-th versifier input goes to sonar.
			versifierSonarIter++
			if *flagSonar && versifierSonarIter%100 == 0 {
				sonar := s.testInputSonar(data, 0)
				s.processSonarData(data, sonar, 0, false)
			} else {
				s.testInput(data, 0, execVersifier)
			}
		}
	}
	s.shutdown()
}

// triageInput processes every new input.
// It calculates per-input metrics like execution time, coverage mask,
// and minimizes the input to the minimal input with the same coverage.
func (s *Slave) triageInput(input MasterInput) {
	if len(input.Data) > MaxInputSize {
		input.Data = input.Data[:MaxInputSize]
	}
	inp := Input{
		data:     input.Data,
		depth:    int(input.Prio),
		typ:      input.Type,
		execTime: 1 << 60,
	}
	// Calculate min exec time, min coverage and max result of 3 runs.
	for i := 0; i < 3; i++ {
		s.execs[execTriageInput]++
		res, ns, cover, _, output, crashed, hanged := s.coverBin.test(inp.data)
		if crashed {
			// Inputs in corpus should not crash.
			s.noteCrasher(inp.data, output, hanged)
			return
		}
		if inp.cover == nil {
			inp.cover = make([]byte, CoverSize)
			copy(inp.cover, cover)
		} else {
			for i, v := range cover {
				x := inp.cover[i]
				if v > x {
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
	if !input.Minimized {
		inp.mine = true
		ro := s.hub.ro.Load().(*ROData)
		// When minimizing new inputs we don't pursue exactly the same coverage,
		// instead we pursue just the "novelty" in coverage.
		// Here we use corpusCover, because maxCover already includes the input coverage.
		newCover, ok := findNewCover(ro.corpusCover, inp.cover)
		if !ok {
			return // covered by somebody else
		}
		inp.data = s.minimizeInput(inp.data, false, func(candidate, cover, output []byte, res int, crashed, hanged bool) bool {
			if crashed {
				s.noteCrasher(candidate, output, hanged)
				return false
			}
			if inp.res != res || worseCover(newCover, cover) {
				s.noteNewInput(candidate, cover, res, inp.depth+1, execMinimizeInput)
				return false
			}
			return true
		})
	} else if !input.Smashed {
		s.smash(inp.data, inp.depth)
	}
	inp.coverSize = 0
	for _, v := range inp.cover {
		if v != 0 {
			inp.coverSize++
		}
	}
	s.hub.newInputC <- inp
}

// processCrasher minimizes new crashers and sends them to the hub.
func (s *Slave) processCrasher(crash NewCrasherArgs) {
	// Hanging inputs can take very long time to minimize.
	if !crash.Hanging {
		crash.Data = s.minimizeInput(crash.Data, true, func(candidate, cover, output []byte, res int, crashed, hanged bool) bool {
			if !crashed {
				return false
			}
			supp := extractSuppression(output)
			if hanged || !bytes.Equal(crash.Suppression, supp) {
				s.noteCrasher(candidate, output, hanged)
				return false
			}
			crash.Error = output
			return true
		})
	}
	s.hub.newCrasherC <- crash
}

// minimizeInput applies series of minimizing transformations to data
// and asks pred whether the input is equivalent to the original one or not.
func (s *Slave) minimizeInput(data []byte, canonicalize bool, pred func(candidate, cover, output []byte, result int, crashed, hanged bool) bool) []byte {
	res := make([]byte, len(data))
	copy(res, data)
	start := time.Now()
	stat := &s.execs[execMinimizeInput]
	if canonicalize {
		stat = &s.execs[execMinimizeCrasher]
	}

	// First, try to cut tail.
	for n := 1024; n != 0; n /= 2 {
		for len(res) > n {
			if time.Since(start) > *flagMinimize {
				return res
			}
			candidate := res[:len(res)-n]
			*stat++
			result, _, cover, _, output, crashed, hanged := s.coverBin.test(candidate)
			if !pred(candidate, cover, output, result, crashed, hanged) {
				break
			}
			res = candidate
		}
	}

	// Then, try to remove each individual byte.
	tmp := make([]byte, len(res))
	for i := 0; i < len(res); i++ {
		if time.Since(start) > *flagMinimize {
			return res
		}
		candidate := tmp[:len(res)-1]
		copy(candidate[:i], res[:i])
		copy(candidate[i:], res[i+1:])
		*stat++
		result, _, cover, _, output, crashed, hanged := s.coverBin.test(candidate)
		if !pred(candidate, cover, output, result, crashed, hanged) {
			continue
		}
		res = makeCopy(candidate)
		i--
	}

	// Then, try to remove each possible subset of bytes.
	for i := 0; i < len(res)-1; i++ {
		copy(tmp, res[:i])
		for j := len(res); j > i+1; j-- {
			if time.Since(start) > *flagMinimize {
				return res
			}
			candidate := tmp[:len(res)-j+i]
			copy(candidate[i:], res[j:])
			*stat++
			result, _, cover, _, output, crashed, hanged := s.coverBin.test(candidate)
			if !pred(candidate, cover, output, result, crashed, hanged) {
				continue
			}
			res = makeCopy(candidate)
			j = len(res)
		}
	}

	// Then, try to replace each individual byte with '0'.
	if canonicalize {
		for i := 0; i < len(res); i++ {
			if res[i] == '0' {
				continue
			}
			if time.Since(start) > *flagMinimize {
				return res
			}
			candidate := tmp[:len(res)]
			copy(candidate, res)
			candidate[i] = '0'
			*stat++
			result, _, cover, _, output, crashed, hanged := s.coverBin.test(candidate)
			if !pred(candidate, cover, output, result, crashed, hanged) {
				continue
			}
			res = makeCopy(candidate)
		}
	}

	return res
}

// smash gives some minimal attention to every new input.
func (s *Slave) smash(data []byte, depth int) {
	ro := s.hub.ro.Load().(*ROData)

	// Pass it through sonar.
	if *flagSonar {
		sonar := s.testInputSonar(data, depth)
		s.processSonarData(data, sonar, depth, true)
	}

	// Flip each bit one-by-one.
	for i := 0; i < len(data)*8; i++ {
		data[i/8] ^= 1 << uint(i%8)
		s.testInput(data, depth, execSmash)
		data[i/8] ^= 1 << uint(i%8)
	}

	// Two walking bits.
	for i := 0; i < len(data)*8-1; i++ {
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
		s.testInput(data, depth, execSmash)
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
	}

	// Four walking bits.
	for i := 0; i < len(data)*8-3; i++ {
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
		data[(i+2)/8] ^= 1 << uint((i+2)%8)
		data[(i+3)/8] ^= 1 << uint((i+3)%8)
		s.testInput(data, depth, execSmash)
		data[i/8] ^= 1 << uint(i%8)
		data[(i+1)/8] ^= 1 << uint((i+1)%8)
		data[(i+2)/8] ^= 1 << uint((i+2)%8)
		data[(i+3)/8] ^= 1 << uint((i+3)%8)
	}

	// Byte flip.
	for i := 0; i < len(data); i++ {
		data[i] ^= 0xff
		s.testInput(data, depth, execSmash)
		data[i] ^= 0xff
	}

	// Two walking bytes.
	for i := 0; i < len(data)-1; i++ {
		data[i] ^= 0xff
		data[i+1] ^= 0xff
		s.testInput(data, depth, execSmash)
		data[i] ^= 0xff
		data[i+1] ^= 0xff
	}

	// Four walking bytes.
	for i := 0; i < len(data)-3; i++ {
		data[i] ^= 0xff
		data[i+1] ^= 0xff
		data[i+2] ^= 0xff
		data[i+3] ^= 0xff
		s.testInput(data, depth, execSmash)
		data[i] ^= 0xff
		data[i+1] ^= 0xff
		data[i+2] ^= 0xff
		data[i+3] ^= 0xff
	}

	// Increment/decrement every byte.
	for i := 0; i < len(data); i++ {
		for j := uint8(1); j <= 4; j++ {
			data[i] += j
			s.testInput(data, depth, execSmash)
			data[i] -= j
			data[i] -= j
			s.testInput(data, depth, execSmash)
			data[i] += j
		}
	}

	// Set bytes to interesting values.
	for i := 0; i < len(data); i++ {
		v := data[i]
		for _, x := range interesting8 {
			data[i] = uint8(x)
			s.testInput(data, depth, execSmash)
		}
		data[i] = v
	}

	// Set words to interesting values.
	for i := 0; i < len(data)-1; i++ {
		p := (*int16)(unsafe.Pointer(&data[i]))
		v := *p
		for _, x := range interesting16 {
			*p = x
			s.testInput(data, depth, execSmash)
			if x != 0 && x != -1 {
				*p = int16(swap16(uint16(x)))
				s.testInput(data, depth, execSmash)
			}
		}
		*p = v
	}

	// Set double-words to interesting values.
	for i := 0; i < len(data)-3; i++ {
		p := (*int32)(unsafe.Pointer(&data[i]))
		v := *p
		for _, x := range interesting32 {
			*p = x
			s.testInput(data, depth, execSmash)
			if x != 0 && x != -1 {
				*p = int32(swap32(uint32(x)))
				s.testInput(data, depth, execSmash)
			}
		}
		*p = v
	}

	// Trim after every byte.
	for i := 1; i < len(data); i++ {
		tmp := data[:i]
		s.testInput(tmp, depth, execSmash)
	}

	// Insert a byte after every byte.
	tmp := make([]byte, len(data)+1)
	if len(tmp) > MaxInputSize {
		tmp = tmp[:MaxInputSize]
	}
	for i := 0; i <= len(data) && i < MaxInputSize-1; i++ {
		copy(tmp, data[:i])
		copy(tmp[i+1:], data[i:])
		tmp[i] = 0
		s.testInput(tmp, depth, execSmash)
		tmp[i] = 'a'
		s.testInput(tmp, depth, execSmash)
	}

	// Do a bunch of random mutations so that this input catches up with the rest.
	for i := 0; i < 1e4; i++ {
		tmp := s.mutator.mutate(data, ro)
		s.testInput(tmp, depth+1, execFuzz)
	}
}

func (s *Slave) testInput(data []byte, depth, typ int) {
	s.testInputImpl(s.coverBin, data, depth, typ)
}

func (s *Slave) testInputSonar(data []byte, depth int) (sonar []byte) {
	return s.testInputImpl(s.sonarBin, data, depth, execSonar)
}

func (s *Slave) testInputImpl(bin *TestBinary, data []byte, depth, typ int) (sonar []byte) {
	ro := s.hub.ro.Load().(*ROData)
	if len(ro.badInputs) > 0 {
		if _, ok := ro.badInputs[hash(data)]; ok {
			return nil // no, thanks
		}
	}
	s.execs[typ]++
	res, _, cover, sonar, output, crashed, hanged := bin.test(data)
	if crashed {
		s.noteCrasher(data, output, hanged)
		return nil
	}
	s.noteNewInput(data, cover, res, depth, typ)
	return sonar
}

func (s *Slave) noteNewInput(data, cover []byte, res, depth, typ int) {
	if res < 0 {
		// User said to not add this input to corpus.
		return
	}
	if s.hub.updateMaxCover(cover) {
		s.triageQueue = append(s.triageQueue, MasterInput{makeCopy(data), uint64(depth), typ, false, false})
	}
}

func (s *Slave) noteCrasher(data, output []byte, hanged bool) {
	ro := s.hub.ro.Load().(*ROData)
	supp := extractSuppression(output)
	if _, ok := ro.suppressions[hash(supp)]; ok {
		return
	}
	s.crasherQueue = append(s.crasherQueue, NewCrasherArgs{
		Data:        makeCopy(data),
		Error:       output,
		Suppression: supp,
		Hanging:     hanged,
	})
}

func (s *Slave) periodicCheck() {
	if atomic.LoadUint32(&shutdown) != 0 {
		s.shutdown()
		select {}
	}
	if time.Since(s.lastSync) < syncPeriod {
		return
	}
	s.execs[execTotal] += s.stats.execs
	s.lastSync = time.Now()
	s.hub.syncC <- s.stats
	s.stats.execs = 0
	s.stats.restarts = 0
	if *flagV >= 2 {
		log.Printf("slave %v: triageq=%v execs=%v mininp=%v mincrash=%v triage=%v fuzz=%v versifier=%v smash=%v sonar=%v hint=%v",
			s.id, len(s.triageQueue),
			s.execs[execTotal], s.execs[execMinimizeInput], s.execs[execMinimizeCrasher],
			s.execs[execTriageInput], s.execs[execFuzz], s.execs[execVersifier], s.execs[execSmash],
			s.execs[execSonar], s.execs[execSonarHint])
	}
}

// shutdown cleanups after slave, it is not guaranteed to be called.
func (s *Slave) shutdown() {
	s.coverBin.close()
	s.sonarBin.close()
}

func extractSuppression(out []byte) []byte {
	var supp []byte
	seenPanic := false
	collect := false
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := s.Text()
		if !seenPanic && (strings.HasPrefix(line, "panic: ") ||
			strings.HasPrefix(line, "fatal error: ") ||
			strings.HasPrefix(line, "SIG") && strings.Index(line, ": ") != 0) {
			// Start of a crash message.
			seenPanic = true
			supp = append(supp, line...)
			supp = append(supp, '\n')
			if line == "SIGABRT: abort" || line == "signal: killed" {
				return supp // timeout stacks are flaky
			}
		}
		if collect && line == "runtime stack:" {
			// Skip runtime stack.
			// Unless it is a runtime bug, user stack is more descriptive.
			collect = false
		}
		if collect && len(line) > 0 && (line[0] >= 'a' && line[0] <= 'z' ||
			line[0] >= 'A' && line[0] <= 'Z') {
			// Function name line.
			idx := strings.LastIndex(line, "(")
			if idx != -1 {
				supp = append(supp, line[:idx]...)
				supp = append(supp, '\n')
			}
		}
		if collect && line == "" {
			// End of first goroutine stack.
			break
		}
		if seenPanic && !collect && line == "" {
			// Start of first goroutine stack.
			collect = true
		}
	}
	if len(supp) == 0 {
		supp = out
	}
	return supp
}

func reverse(data []byte) []byte {
	tmp := make([]byte, len(data))
	for i, v := range data {
		tmp[len(data)-i-1] = v
	}
	return tmp
}

func increment(data []byte) []byte {
	tmp := make([]byte, len(data))
	for i, v := range data {
		tmp[i] = v + 1
		if v != 0xff {
			break
		}
	}
	return tmp
}

func decrement(data []byte) []byte {
	tmp := make([]byte, len(data))
	for i, v := range data {
		tmp[i] = v - 1
		if v != 0 {
			break
		}
	}
	return tmp
}
