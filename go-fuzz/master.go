package main

import (
	"errors"
	"log"
	"net"
	"net/rpc"
	"path/filepath"
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
	suppressions *PersistentSet
	crashers     *PersistentSet

	startTime     time.Time
	lastInput     time.Time
	statExecs     uint64
	statRestarts  uint64
	coverFullness float64
}

type MasterSlave struct {
	id       int
	procs    int
	pending  []MasterInput
	lastSync time.Time
}

func masterMain(ln net.Listener) {
	m := &Master{}

	m.startTime = time.Now()
	m.lastInput = time.Now()
	m.suppressions = newPersistentSet(filepath.Join(*flagWorkdir, "suppressions"))
	m.crashers = newPersistentSet(filepath.Join(*flagWorkdir, "crashers"))
	m.corpus = newPersistentSet(filepath.Join(*flagWorkdir, "corpus"))
	m.bootstrap = newPersistentSet(*flagCorpus)
	if len(m.bootstrap.m) == 0 {
		m.bootstrap.add(Artifact{[]byte{}, 0})
	}

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
		log.Printf("slaves: %v/%v, corpus: %v (%v ago), crashers: %v, suppressions: %v,"+
			" restarts: 1/%v, execs: %v (%.0f/sec), cover: %.2f%%, uptime: %v",
			len(m.slaves), procs, len(m.corpus.m), lastInput, len(m.crashers.m), len(m.suppressions.m),
			restarts, m.statExecs, float64(m.statExecs)*1e9/float64(uptime), m.coverFullness*100, uptime)
		for id, s := range m.slaves {
			if time.Since(s.lastSync) < syncDeadline {
				continue
			}
			log.Printf("slave %v died", s.id)
			delete(m.slaves, id)
		}
		m.mu.Unlock()
	}
}

type ConnectArgs struct {
	Procs int
}

type ConnectRes struct {
	ID     int
	Corpus []MasterInput
}

type MasterInput struct {
	Data      []byte
	Prio      uint64
	Minimized bool
	Smashed   bool
}

func (m *Master) Connect(a *ConnectArgs, r *ConnectRes) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idSeq++
	s := &MasterSlave{
		id:       m.idSeq,
		procs:    a.Procs,
		lastSync: time.Now(),
	}
	m.slaves[s.id] = s
	r.ID = s.id
	for _, a := range m.bootstrap.m {
		r.Corpus = append(r.Corpus, MasterInput{a.data, a.meta, false, false})
	}
	for _, a := range m.corpus.m {
		r.Corpus = append(r.Corpus, MasterInput{a.data, a.meta, true, true})
	}
	return nil
}

type NewInputArgs struct {
	ID   int
	Data []byte
	Prio uint64
}

func (m *Master) NewInput(a *NewInputArgs, r *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.slaves[a.ID]
	if s == nil {
		return errors.New("unknown slave")
	}

	art := Artifact{a.Data, a.Prio}
	if !m.corpus.add(art) {
		return nil
	}
	m.lastInput = time.Now()
	for _, s1 := range m.slaves {
		s1.pending = append(s1.pending, MasterInput{a.Data, a.Prio, true, s1 != s})
	}

	return nil
}

type NewCrasherArgs struct {
	Data        []byte
	Error       []byte
	Suppression []byte
	Hanging     bool
}

func (m *Master) NewCrasher(a *NewCrasherArgs, r *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.suppressions.add(Artifact{a.Suppression, 0}) || !m.crashers.add(Artifact{a.Data, 0}) {
		return nil
	}
	m.crashers.addDescription(a.Data, a.Error, "output")
	return nil
}

type SyncArgs struct {
	ID            int
	Execs         uint64
	Restarts      uint64
	CoverFullness float64
}

type SyncRes struct {
	Inputs []MasterInput
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
	if m.coverFullness < a.CoverFullness {
		m.coverFullness = a.CoverFullness
	}
	s.lastSync = time.Now()
	r.Inputs = s.pending
	s.pending = nil
	return nil
}
