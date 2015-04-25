package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Master manages persistent fuzzer state like input corpus and crashers.
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

// MasterSlave represents master's view of a slave.
type MasterSlave struct {
	id       int
	procs    int
	pending  []MasterInput
	lastSync time.Time
}

// masterMain is entry function for master.
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
		// Nuke dead slaves.
		for id, s := range m.slaves {
			if time.Since(s.lastSync) < syncDeadline {
				continue
			}
			log.Printf("slave %v died", s.id)
			delete(m.slaves, id)
		}

		// Print stats line.
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
		log.Printf("slaves: %v, corpus: %v (%v ago), crashers: %v,"+
			" restarts: 1/%v, execs: %v (%.0f/sec), cover: %.2f%%, uptime: %v",
			procs, len(m.corpus.m), fmtDuration(lastInput), len(m.crashers.m),
			restarts, m.statExecs, float64(m.statExecs)*1e9/float64(uptime),
			m.coverFullness*100, fmtDuration(uptime))
		m.mu.Unlock()
	}
}

func fmtDuration(d time.Duration) string {
	if d.Hours() >= 1 {
		return fmt.Sprintf("%vh%vm", int(d.Hours()), int(d.Minutes())%60)
	} else if d.Minutes() >= 1 {
		return fmt.Sprintf("%vm%vs", int(d.Minutes()), int(d.Seconds())%60)
	} else {
		return fmt.Sprintf("%vs", int(d.Seconds()))
	}
}

type ConnectArgs struct {
	Procs int
}

type ConnectRes struct {
	ID     int
	Corpus []MasterInput
}

// MasterInput is description of input that is passed between master and slave.
type MasterInput struct {
	Data      []byte
	Prio      uint64
	Minimized bool
	Smashed   bool
}

// Connect attaches new slave to master.
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
	// Give the slave initial corpus.
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

// NewInput saves new interesting input on master.
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
	// Queue the input for sending to every slave.
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

// NewCrasher saves new crasher input on master.
func (m *Master) NewCrasher(a *NewCrasherArgs, r *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.suppressions.add(Artifact{a.Suppression, 0}) || !m.crashers.add(Artifact{a.Data, 0}) {
		// Already have this.
		return nil
	}

	// Prepare quoted version of input to simplify creation of standalone reproducers.
	var buf bytes.Buffer
	for i := 0; i < len(a.Data); i += 20 {
		e := i + 20
		if e > len(a.Data) {
			e = len(a.Data)
		}
		fmt.Fprintf(&buf, "\t%q", a.Data[i:e])
		if e != len(a.Data) {
			fmt.Printf(" +")
		}
		fmt.Printf("\n")
	}
	m.crashers.addDescription(a.Data, buf.Bytes(), "quoted")
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
	Inputs []MasterInput // new interesting inputs
}

// Sync is a periodic sync with a slave.
// Slave sends statitstics. Master returns new inputs.
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
