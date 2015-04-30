package main

import (
	"bufio"
	"bytes"
	"index/suffixarray"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	dep "github.com/dvyukov/go-fuzz/go-fuzz-dep"
)

const sonarQueueSize = 10000

// Slave manages one testee.
type Slave struct {
	hub     *Hub
	mutator *Mutator
	id      int

	coverRegion []byte
	inputRegion []byte
	sonarRegion []byte
	commFile    string

	sonarQueue   []MasterInput
	triageQueue  []MasterInput
	crasherQueue []NewCrasherArgs

	lastSync time.Time
	stats    Stats

	testee *Testee
}

type Input struct {
	mine            bool
	data            []byte
	cover           []byte
	coverSize       int
	res             int
	depth           int
	execTime        uint64
	score           int
	runningScoreSum int
}

func slaveMain() {
	hub := newHub()
	for i := 0; i < *flagProcs; i++ {
		s := &Slave{
			hub:     hub,
			mutator: newMutator(),
			id:      i,
		}
		s.setupCommFile()
		go s.loop()
	}
}

func (s *Slave) loop() {
	for iter := 0; atomic.LoadUint32(&shutdown) == 0; iter++ {
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

		select {
		case input := <-s.hub.triageC:
			if *flagV >= 2 {
				log.Printf("slave %v triages master input [%v]%v minimized=%v smashed=%v", s.id, len(input.Data), hash(input.Data), input.Minimized, input.Smashed)
			}
			s.triageInput(input)
			continue
		default:
		}

		if len(s.sonarQueue) > 0 {
			n := len(s.sonarQueue) - 1
			input := s.sonarQueue[n]
			s.sonarQueue[n] = MasterInput{}
			s.sonarQueue = s.sonarQueue[:n]
			if *flagV >= 2 {
				log.Printf("slave %v tests sonar input [%v]%v", s.id, len(input.Data), hash(input.Data))
			}
			s.testInput(input.Data, int(input.Prio))
			continue
		}

		ro := s.hub.ro.Load().(*ROData)
		if len(ro.corpus) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		data, depth := s.mutator.generate(ro)
		sonar := s.testInput(data, depth)
		if iter%100 == 0 && len(s.sonarQueue) < sonarQueueSize/2 {
			s.processSonarData(data, sonar, depth)
		}
	}
	s.shutdown()
}

// triageInput processes every new input.
// It calculates per-input metrics like execution time, coverage mask,
// and minimizes the input to the minimal input with the same coverage.
func (s *Slave) triageInput(input MasterInput) {
	inp := Input{
		data:     input.Data,
		depth:    int(input.Prio),
		execTime: 1 << 60,
	}
	// Calculate min exec time, min coverage and max result of 3 runs.
	for i := 0; i < 3; i++ {
		res, ns, cover, _, output, crashed, hanged := s.exec(inp.data)
		if crashed {
			// Inputs in corpus should not crash.
			s.noteCrasher(inp.data, output, hanged)
			return
		}
		if inp.cover == nil {
			inp.cover = make([]byte, dep.CoverSize)
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
	inp.coverSize = 0
	for _, v := range inp.cover {
		if v != 0 {
			inp.coverSize++
		}
	}
	if !input.Minimized {
		inp.mine = true
		inp.data = s.minimizeInput(inp.data, false, func(candidate, cover, output []byte, res int, crashed, hanged bool) bool {
			if crashed {
				s.noteCrasher(candidate, output, hanged)
				return false
			}
			if inp.res != res || !bytes.Equal(inp.cover, cover) {
				// TODO: this can be a new intersting input.
				return false
			}
			return true
		})
	} else if !input.Smashed {
		s.smash(inp.data, inp.depth)
	}
	s.hub.newInputC <- inp
}

// processCrasher minimizes new crashers and sends them to the hub.
func (s *Slave) processCrasher(crash NewCrasherArgs) {
	ro := s.hub.ro.Load().(*ROData)
	if _, ok := ro.suppressions[hash(crash.Suppression)]; ok {
		return
	}
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

	// First, try to cut tail.
	for n := 1024; n != 0; n /= 2 {
		for len(res) > n {
			if time.Since(start) > *flagMinimize {
				return res
			}
			candidate := res[:len(res)-n]
			result, _, cover, _, output, crashed, hanged := s.exec(candidate)
			if !pred(candidate, cover, output, result, crashed, hanged) {
				break
			}
			res = candidate
		}
	}

	// Then, try to remove each individual byte.
	tmp := make([]byte, len(res))
	for i := 0; i < len(res); i++ {
		candidate := tmp[:len(res)-1]
		copy(candidate[:i], res[:i])
		copy(candidate[i:], res[i+1:])
		result, _, cover, _, output, crashed, hanged := s.exec(candidate)
		if !pred(candidate, cover, output, result, crashed, hanged) {
			continue
		}
		res = make([]byte, len(candidate))
		copy(res, candidate)
		if time.Since(start) > *flagMinimize {
			return res
		}
		i--
	}

	// Then, try to replace each individual byte with '0'.
	if canonicalize {
		for i := 0; i < len(res); i++ {
			if res[i] == '.' {
				continue
			}
			candidate := tmp[:len(res)]
			copy(candidate, res)
			candidate[i] = '0'
			result, _, cover, _, output, crashed, hanged := s.exec(candidate)
			if !pred(candidate, cover, output, result, crashed, hanged) {
				continue
			}
			res = make([]byte, len(candidate))
			copy(res, candidate)
			if time.Since(start) > *flagMinimize {
				return res
			}
		}
	}

	return res
}

func (s *Slave) processSonarData(data, sonar []byte, depth int) {
	if len(s.sonarQueue) >= sonarQueueSize {
		return
	}

	//log.Printf("got %v bytes of sonar data\n", len(sonar))
	//d := len(sonar)
	//n := 0
	q := 0
	for len(sonar) > dep.SonarHdrLen {
		flags := sonar[0]
		n1 := sonar[1]
		n2 := sonar[2]
		sonar = sonar[dep.SonarHdrLen:]
		if n1 == 0 || n1 > dep.SonarMaxLen || n2 == 0 || n2 > dep.SonarMaxLen || len(sonar) < int(n1)+int(n2) {
			log.Fatalf("corrputed sonar data: hdr=[%v/%v/%v] data=%v", flags, n1, n2, len(sonar))
		}
		//n++
		v1 := sonar[:n1]
		v2 := sonar[n1 : n1+n2]
		sonar = sonar[n1+n2:]
		if flags&dep.SonarString == 0 {
			//l1, l2 := len(v1), len(v2)
			for len(v1) > 0 || len(v2) > 0 {
				i := len(v1) - 1
				if len(v2) > len(v1) {
					i = len(v2) - 1
				}
				var c1, c2 byte
				if i < len(v1) {
					c1 = v1[i]
				}
				if i < len(v2) {
					c2 = v2[i]
				}
				if (c1 == 0 || c1 == 0xff) && (c2 == 0 || c2 == 0xff) {
					if i < len(v1) {
						v1 = v1[:i]
					}
					if i < len(v2) {
						v2 = v2[:i]
					}
					continue
				}
				break
			}
			/*
				if l1 != len(v1) || l2 != len(v2) {
					log.Printf("REDUCE: [%v]%q -> [%v]%q, [%v]%q -> [%v]%q", l1, v1[:l1], len(v1), v1, l2, v2[:l2], len(v2), v2)
				} else {
					log.Printf("NOT REDUCED: [%v]%q, [%v]%q", len(v1), v1, len(v2), v2)
				}
			*/
		}
		if bytes.Equal(v1, v2) {
			continue
		}
		if len(v1) == 0 || len(v2) == 0 {
			// TODO: it still can be interesting for string operations.
			// E.g. for fld == "" we could find the string and remove it,
			// (potentially altering preceeding length field).
			continue
		}
		checked := make(map[string]struct{})
		//log.Printf("  sonar: flags=%v %q vs %q\n", flags, v1, v2)
		check := func(v1, v2 []byte) {
			if len(v1) == 0 {
				return
			}
			vv := string(v1) + "\t\t\t" + string(v2)
			if _, ok := checked[vv]; ok {
				return
			}
			checked[vv] = struct{}{}
			pos := 0
			for q < 1000 && len(s.sonarQueue) < sonarQueueSize {
				i := bytes.Index(data[pos:], v1)
				if i == -1 {
					break
				}
				i += pos
				pos = i + 1
				tmp := make([]byte, len(data)-len(v1)+len(v2))
				copy(tmp, data[:i])
				copy(tmp[i:], v2)
				copy(tmp[i+len(v2):], data[i+len(v1):])
				//log.Printf("sonar(%q,%q): %q -> %q\n", v1, v2, data, tmp)
				s.sonarQueue = append(s.sonarQueue, MasterInput{Data: tmp, Prio: uint64(depth + 1)})
				q++
			}
		}
		if flags&dep.SonarConst1 == 0 {
			check(v1, v2)
			if flags&dep.SonarString == 0 && len(v1) > 1 {
				check(reverse(v1), reverse(v2))
			}
		}
		if flags&dep.SonarConst2 == 0 {
			check(v2, v1)
			if flags&dep.SonarString == 0 && len(v2) > 1 {
				check(reverse(v2), reverse(v1))
			}
		}
	}
	//log.Printf("sonar: data=%v n=%v q=%v\n", d, n, q)
}

// smash gives some minimal attention to every new input.
func (s *Slave) smash(data []byte, depth int) {
	ro := s.hub.ro.Load().(*ROData)

	// TODO: some of the mutations are disabled, because they take too long
	// at least during experimentation (but most likely ok for real runs).
	// Figure out what to do here.

	sonar := s.testInput(data, depth)
	s.processSonarData(data, sonar, depth)

	suffix := suffixarray.New(data)
	/*
		for i0, lit := range ro.strLits {
			for _, pos := range suffix.Lookup(lit, -1) {
				for i1, lit1 := range ro.strLits {
					if i0 == i1 {
						continue
					}
					tmp := make([]byte, len(data)-len(lit)+len(lit1))
					copy(tmp, data[:pos])
					copy(tmp[pos:], lit1)
					copy(tmp[pos+len(lit1):], data[pos+len(lit):])
					s.testInput(tmp, depth)
				}
			}
		}
	*/
	for i0, lit := range ro.intLits {
		for _, pos := range suffix.Lookup(lit, -1) {
			for i1, lit1 := range ro.intLits {
				if i0 == i1 || len(lit) != len(lit1) {
					continue
				}
				tmp := make([]byte, len(lit))
				copy(tmp, data[pos:])
				copy(data[pos:], lit1)
				s.testInput(data, depth)
				copy(data[pos:], tmp)
			}
		}

		if len(lit) == 1 {
			continue
		}
		lit = reverse(lit)
		for _, pos := range suffix.Lookup(lit, -1) {
			for i1, lit1 := range ro.intLits {
				if i0 == i1 || len(lit) != len(lit1) {
					continue
				}
				lit1 = reverse(lit1)
				tmp := make([]byte, len(lit))
				copy(tmp, data[pos:])
				copy(data[pos:], lit1)
				s.testInput(data, depth)
				copy(data[pos:], tmp)
			}
		}
	}

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

	// Insert a byte after every byte.
	tmp := make([]byte, len(data)+1)
	for i := 0; i <= len(data); i++ {
		copy(tmp, data[:i])
		copy(tmp[i+1:], data[i:])
		tmp[i] = 0
		s.testInput(tmp, depth)
		tmp[i] = 'a'
		s.testInput(tmp, depth)
	}

	// Do a bunch of random mutations so that this input catches up with the rest.
	for i := 0; i < 1e4; i++ {
		tmp := s.mutator.mutate(data, ro)
		s.testInput(tmp, depth+1)
	}
}

func (s *Slave) testInput(data []byte, depth int) (sonar []byte) {
	ro := s.hub.ro.Load().(*ROData)
	if len(ro.badInputs) > 0 {
		if _, ok := ro.badInputs[hash(data)]; ok {
			return nil // no, thanks
		}
	}
	_, _, cover, sonar, output, crashed, hanged := s.exec(data)
	if crashed {
		s.noteCrasher(data, output, hanged)
		return nil
	}
	newCover, newCount := compareCover(ro.maxCover, cover)
	if !newCover && !newCount {
		return sonar
	}
	// TODO: give more priority for newCover
	s.triageQueue = append(s.triageQueue, MasterInput{append([]byte{}, data...), uint64(depth), false, false})
	return sonar
}

func (s *Slave) exec(data []byte) (res int, ns uint64, cover, sonar, output []byte, crashed, hanged bool) {
	for {
		// This is the only function that is executed regularly,
		// so we tie some periodic checks to it.
		s.periodicCheck()

		s.stats.execs++
		if s.testee == nil {
			s.stats.restarts++
			s.testee = newTestee(*flagBin, s.commFile, s.coverRegion, s.inputRegion, s.sonarRegion)
		}
		var retry bool
		res, ns, cover, sonar, crashed, hanged, retry = s.testee.test(data)
		if retry {
			s.testee.shutdown()
			s.testee = nil
			continue
		}
		if crashed {
			output = s.testee.shutdown()
			s.testee = nil
			return
		}
		return
	}
}

func (s *Slave) noteCrasher(data, output []byte, hanged bool) {
	s.crasherQueue = append(s.crasherQueue, NewCrasherArgs{
		Data:        data,
		Error:       output,
		Suppression: extractSuppression(output),
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
	s.lastSync = time.Now()
	s.hub.syncC <- s.stats
	s.stats.execs = 0
	s.stats.restarts = 0
}

// shutdown cleanups after slave, it is not guaranteed to be called.
func (s *Slave) shutdown() {
	if s.testee != nil {
		s.testee.shutdown()
		s.testee = nil
	}
	os.Remove(s.commFile)
}

func (s *Slave) setupCommFile() {
	comm, err := ioutil.TempFile("", "go-fuzz-comm")
	if err != nil {
		log.Fatalf("failed to create comm file: %v", err)
	}
	comm.Truncate(dep.CoverSize + dep.MaxInputSize + dep.SonarRegionSize)
	comm.Close()
	s.commFile = comm.Name()
	fd, err := syscall.Open(comm.Name(), syscall.O_RDWR, 0)
	if err != nil {
		log.Fatalf("failed to open comm file: %v", err)
	}
	mem, err := syscall.Mmap(fd, 0, dep.CoverSize+dep.MaxInputSize+dep.SonarRegionSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		log.Fatalf("failed to mmap comm file: %v", err)
	}
	s.coverRegion = mem[:dep.CoverSize]
	s.inputRegion = mem[dep.CoverSize : dep.CoverSize+dep.SonarRegionSize]
	s.sonarRegion = mem[dep.CoverSize+dep.SonarRegionSize:]
}

func compareCover(base, cur []byte) (bool, bool) {
	if len(base) != dep.CoverSize || len(cur) != dep.CoverSize {
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

func extractSuppression(out []byte) []byte {
	var supp []byte
	seenPanic := false
	collect := false
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		// TODO: make clear when it is a timeout.
		line := s.Text()
		if !seenPanic && (strings.HasPrefix(line, "panic: ") ||
			strings.HasPrefix(line, "fatal error: ") ||
			strings.HasPrefix(line, "SIG") && strings.Index(line, ": ") != 0) {
			// Start of a crash message.
			seenPanic = true
			supp = append(supp, line...)
			supp = append(supp, '\n')
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
