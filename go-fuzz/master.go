package main

import (
	"bufio"
	"bytes"
	"errors"
	"log"
	"net"
	"net/rpc"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Master struct {
	mu           sync.Mutex
	idSeq        int
	slaves       map[int]*MasterSlave
	bootstrap    *PersistentSet
	corpus       *PersistentSet
	fresh        *PersistentSet
	suppressions *PersistentSet
	crashers     *PersistentSet

	startTime    time.Time
	lastInput    time.Time
	statExecs    uint64
	statRestarts uint64
}

type MasterSlave struct {
	id       int
	procs    int
	pending  []MasterInput
	smashing *Artifact
	lastSync time.Time
}

func masterMain(ln net.Listener) {
	m := &Master{}

	m.startTime = time.Now()
	m.lastInput = time.Now()
	m.fresh = newPersistentSet(filepath.Join(*flagWorkdir, "fresh"))
	m.suppressions = newPersistentSet(filepath.Join(*flagWorkdir, "suppressions"))
	m.crashers = newPersistentSet(filepath.Join(*flagWorkdir, "crashers"))
	m.bootstrap = newPersistentSet(*flagCorpus)
	m.corpus = newPersistentSet(filepath.Join(*flagWorkdir, "corpus"))

	m.slaves = make(map[int]*MasterSlave)
	go masterLoop(m)

	s := rpc.NewServer()
	s.Register(m)
	s.Accept(ln)
}

func masterLoop(m *Master) {
	for range time.NewTicker(3 * time.Second).C {
		if atomic.LoadUint32(&shutdown) != 0 {
			return
		}
		m.mu.Lock()
		uptime := time.Since(m.startTime)
		lastInput := time.Since(m.lastInput)
		restarts := uint64(0)
		if m.statExecs != 0 {
			restarts = m.statExecs / m.statRestarts
		}
		procs := 0
		for _, s := range m.slaves {
			procs += s.procs
		}
		log.Printf("slaves: %v/%v, corpus: %v (%v ago), fresh: %v, crashers: %v, suppressions: %v, restarts: 1/%v, execs: %v (%.0f/sec), uptime: %v",
			len(m.slaves), procs, len(m.corpus.m), lastInput, len(m.fresh.m), len(m.crashers.m), len(m.suppressions.m),
			restarts, m.statExecs, float64(m.statExecs)*1e9/float64(uptime), uptime)
		for id, s := range m.slaves {
			if time.Since(s.lastSync) < syncDeadline {
				continue
			}
			log.Printf("slave %v died", s.id)
			delete(m.slaves, id)
			if s.smashing != nil {
				// The slave was smashing a new input.
				// Add the input back to the fresh list,
				// so that another slave can pick it up.
				m.fresh.add(*s.smashing)
			}
		}
		m.mu.Unlock()
	}
}

type ConnectArgs struct {
	Procs int
}

type ConnectRes struct {
	ID        int
	Bootstrap []MasterInput
	Corpus    []MasterInput
}

type MasterInput struct {
	Data []byte
	Prio uint64
}

func (m *Master) Connect(a *ConnectArgs, r *ConnectRes) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idSeq++
	s := &MasterSlave{id: m.idSeq, procs: a.Procs}
	s.lastSync = time.Now()
	m.slaves[s.id] = s

	r.ID = s.id
	for _, a := range m.bootstrap.m {
		r.Bootstrap = append(r.Bootstrap, MasterInput{a.data, a.meta})
	}
	for _, a := range m.corpus.m {
		r.Corpus = append(r.Corpus, MasterInput{a.data, a.meta})
	}
	return nil
}

type NewInputArgs struct {
	Input MasterInput
}

func (m *Master) NewInput(a *NewInputArgs, r *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	art := Artifact{a.Input.Data, a.Input.Prio}
	if !m.corpus.add(art) {
		return nil
	}
	m.fresh.add(art)
	for _, s := range m.slaves {
		s.pending = append(s.pending, a.Input)
	}
	m.lastInput = time.Now()

	return nil
}

type NewCrasherArgs struct {
	Data  []byte
	Error []byte
}

func (m *Master) NewCrasher(a *NewCrasherArgs, r *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	supp := extractSuppression(a.Error)
	if !m.suppressions.add(Artifact{supp, 0}) || !m.crashers.add(Artifact{a.Data, 0}) {
		return nil
	}
	m.crashers.addDescription(a.Data, a.Error, "output")
	return nil
}

type SyncArgs struct {
	ID       int
	Execs    uint64
	Restarts uint64
}

type SyncRes struct {
	Inputs []MasterInput
	Smash  MasterInput
}

func (m *Master) Sync(a *SyncArgs, r *SyncRes) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.slaves[a.ID]
	if s == nil {
		return errors.New("unknown slave")
	}
	m.statExecs += a.Execs
	m.statRestarts += a.Restarts
	s.lastSync = time.Now()
	r.Inputs = s.pending
	s.pending = nil
	if s.smashing == nil && len(m.fresh.m) > 0 {
		input := m.fresh.remove()
		s.smashing = &input
		r.Smash = MasterInput{input.data, input.meta}
	}
	return nil
}

type DoneSmashingArgs struct {
	ID    int
	Input MasterInput
}

type DoneSmashingRes struct {
	Smash MasterInput
}

func (m *Master) DoneSmashing(a *DoneSmashingArgs, r *DoneSmashingRes) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.slaves[a.ID]
	if s == nil {
		return errors.New("unknown slave")
	}
	if s.smashing == nil {
		panic("bad")
	}
	s.smashing = nil
	m.fresh.removePersistent(Artifact{a.Input.Data, a.Input.Prio})
	if len(m.fresh.m) > 0 {
		input := m.fresh.remove()
		s.smashing = &input
		r.Smash = MasterInput{input.data, input.meta}
	}
	return nil
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
		}
		if collect && line == "runtime stack:" {
			// Skip runtime stack.
			// Unless it is a runtime bug, user stack is more descriptive.
			collect = false
		}
		if collect && len(line) > 0 && (line[0] >= 'a' && line[0] <= 'z' ||
			line[0] >= 'A' && line[0] <= 'Z') {
			// Function name line.
			idx := strings.IndexByte(line, '(')
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
