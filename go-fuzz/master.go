// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/rpc"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dvyukov/go-fuzz/go-fuzz/internal/writerset"
)

// Master manages persistent fuzzer state like input corpus and crashers.
type Master struct {
	mu           sync.Mutex
	idSeq        int
	slaves       map[int]*MasterSlave
	corpus       *PersistentSet
	suppressions *PersistentSet
	crashers     *PersistentSet

	startTime     time.Time
	lastInput     time.Time
	statExecs     uint64
	statRestarts  uint64
	coverFullness int

	statsWriters *writerset.WriterSet
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
	m.statsWriters = writerset.New()
	m.startTime = time.Now()
	m.lastInput = time.Now()
	m.suppressions = newPersistentSet(filepath.Join(*flagWorkdir, "suppressions"))
	m.crashers = newPersistentSet(filepath.Join(*flagWorkdir, "crashers"))
	m.corpus = newPersistentSet(filepath.Join(*flagWorkdir, "corpus"))
	if len(m.corpus.m) == 0 {
		m.corpus.add(Artifact{[]byte{}, 0, false})
	}

	m.slaves = make(map[int]*MasterSlave)
	masterListen(m)

	go masterLoop(m)

	s := rpc.NewServer()
	s.Register(m)
	s.Accept(ln)
}

func masterListen(m *Master) {
	if *flagHTTP != "" {
		http.HandleFunc("/eventsource", m.eventSource)
		http.HandleFunc("/", m.index)

		go func() {
			fmt.Printf("Serving statistics on http://%s/\n", *flagHTTP)
			panic(http.ListenAndServe(*flagHTTP, nil))
		}()
	} else {
		runtime.MemProfileRate = 0
	}
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
		m.mu.Unlock()

		m.broadcastStats()
	}
}

func (m *Master) broadcastStats() {
	stats := m.masterStats()

	// log to stdout
	log.Println(stats.String())

	// write to any http clients
	b, err := json.Marshal(stats)
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(m.statsWriters, "event: ping\ndata: %s\n\n", string(b))
	m.statsWriters.Flush()
}

func (m *Master) eventSource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	<-m.statsWriters.Add(w)
}

func (m *Master) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		r.URL.Path = "/stats.html"
	}
	http.FileServer(assetFS()).ServeHTTP(w, r)
}

func (m *Master) masterStats() masterStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := masterStats{
		Corpus:           uint64(len(m.corpus.m)),
		Crashers:         uint64(len(m.crashers.m)),
		Uptime:           fmtDuration(time.Since(m.startTime)),
		StartTime:        m.startTime,
		LastNewInputTime: m.lastInput,
		Execs:            m.statExecs,
		Cover:            uint64(m.coverFullness),
	}

	// Print stats line.
	if m.statExecs != 0 {
		stats.RestartsDenom = m.statExecs / m.statRestarts
	}

	for _, s := range m.slaves {
		stats.Slaves += uint64(s.procs)
	}

	return stats
}

type masterStats struct {
	Slaves, Corpus, Crashers, Execs, Cover, RestartsDenom uint64
	LastNewInputTime, StartTime                           time.Time
	Uptime                                                string
}

func (s masterStats) String() string {
	return fmt.Sprintf("slaves: %v, corpus: %v (%v ago), crashers: %v,"+
		" restarts: 1/%v, execs: %v (%.0f/sec), cover: %v, uptime: %v",
		s.Slaves, s.Corpus, fmtDuration(time.Since(s.LastNewInputTime)),
		s.Crashers, s.RestartsDenom, s.Execs, s.ExecsPerSec(), s.Cover,
		s.Uptime,
	)
}

func (s masterStats) ExecsPerSec() float64 {
	return float64(s.Execs) * 1e9 / float64(time.Since(s.StartTime))
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
	Type      int
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
	for _, a := range m.corpus.m {
		r.Corpus = append(r.Corpus, MasterInput{a.data, a.meta, execCorpus, !a.user, true})
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

	art := Artifact{a.Data, a.Prio, false}
	if !m.corpus.add(art) {
		return nil
	}
	m.lastInput = time.Now()
	// Queue the input for sending to every slave.
	for _, s1 := range m.slaves {
		s1.pending = append(s1.pending, MasterInput{a.Data, a.Prio, execCorpus, true, s1 != s})
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

	if !*flagDup && !m.suppressions.add(Artifact{a.Suppression, 0, false}) {
		return nil // Already have this.
	}
	if !m.crashers.add(Artifact{a.Data, 0, false}) {
		return nil // Already have this.
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
			fmt.Fprintf(&buf, " +")
		}
		fmt.Fprintf(&buf, "\n")
	}
	m.crashers.addDescription(a.Data, buf.Bytes(), "quoted")
	m.crashers.addDescription(a.Data, a.Error, "output")

	return nil
}

type SyncArgs struct {
	ID            int
	Execs         uint64
	Restarts      uint64
	CoverFullness int
}

type SyncRes struct {
	Inputs []MasterInput // new interesting inputs
}

var errUnkownSlave = errors.New("unknown slave")

// Sync is a periodic sync with a slave.
// Slave sends statistics. Master returns new inputs.
func (m *Master) Sync(a *SyncArgs, r *SyncRes) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.slaves[a.ID]
	if s == nil {
		return errUnkownSlave
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
