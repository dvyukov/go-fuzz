package main

import (
	"crypto/sha1"
	"encoding/hex"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type driver struct {
	Mu        sync.Mutex
	Addr      string
	IdSeq     int
	Procs     int
	Workers   map[int]*worker
	Corpus    map[string]bool
	CorpusDir string
	Known     map[string]bool
	KnownDir  string
	BugDir    string
}

type worker struct {
	Id         int
	Cmd        *exec.Cmd
	RescueFile string
	Pending    []string
	LastPing   time.Time
}

func master(ln net.Listener) {
	type Driver struct {
		*driver
	}
	s := rpc.NewServer()
	s.Register(&Driver{newDriver(ln.Addr().String())})
	s.Accept(ln)
}

func newDriver(addr string) *driver {
	d := &driver{Addr: addr}
	d.Procs = *flagProcs
	if d.Procs <= 0 {
		d.Procs = runtime.NumCPU()
	}

	d.Corpus = make(map[string]bool)
	d.CorpusDir = filepath.Join(*flagWorkdir, "corpus")
	os.MkdirAll(d.CorpusDir, 0770)
	if *flagCorpus != "" {
		d.readInDir(d.Corpus, *flagCorpus)
	}
	d.readInDir(d.Corpus, d.CorpusDir)
	if len(d.Corpus) == 0 {
		d.Corpus[""] = true
	}

	d.Known = make(map[string]bool)
	d.KnownDir = filepath.Join(*flagWorkdir, "known")
	os.MkdirAll(d.KnownDir, 0770)
	d.readInDir(d.Known, d.KnownDir)

	d.BugDir = filepath.Join(*flagWorkdir, "bugs")
	os.MkdirAll(d.BugDir, 0770)

	log.Printf("corpus contains %v inputs, know %v bugs\n", len(d.Corpus), len(d.Known))

	d.Workers = make(map[int]*worker)
	go d.loop()
	return d
}

func (d *driver) readInDir(m map[string]bool, dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("error during corpus walk: %v\n", err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("error during corpus read: %v\n", err)
			return nil
		}
		m[string(data)] = true
		return nil
	})
}

func (d *driver) loop() {
	/*
		for range time.NewTicker(time.Second).C {
			d.Mu.Lock()
			for _, w := range d.Workers {
				if time.Since(w.LastPing) > time.Minute {
					log.Printf("worker %v hang, killing", w.Id)
					w.Cmd.Process.Kill()
				}
			}
			if len(d.Workers) < d.Procs {
				w := d.newWorker()
			}
			d.Mu.Unlock()
		}
	*/
}

func (d *driver) saveFile(dir string, str string) {
	data := []byte(str)
	h := sha1.New()
	h.Write(data)
	fname := filepath.Join(dir, hex.EncodeToString(h.Sum(nil)[:]))
	ioutil.WriteFile(fname, data, 0660)
}

func (d *driver) newWorker() *worker {
	d.IdSeq++
	w := &worker{Id: d.IdSeq}
	w.LastPing = time.Now()
	d.Workers[w.Id] = w
	return w
}

type InitArgs struct {
}

type InitRes struct {
	Id     int
	Corpus []string
}

func (d *driver) Init(a *InitArgs, r *InitRes) error {
	//log.Printf("Init: %v", a.Id)
	d.Mu.Lock()
	defer d.Mu.Unlock()
	w := d.newWorker()
	r.Id = w.Id
	for data := range d.Corpus {
		r.Corpus = append(r.Corpus, data)
	}
	return nil
}

type NewInputArgs struct {
	Id   int
	Data string
}

func (d *driver) NewInput(a *NewInputArgs, r *int) error {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	if d.Corpus[a.Data] {
		return nil
	}
	d.Corpus[a.Data] = true
	d.saveFile(d.CorpusDir, a.Data)

	for _, w := range d.Workers {
		w.Pending = append(w.Pending, a.Data)
	}

	data := []byte(a.Data)
	if len(data) > 50 {
		data = data[:50]
	}
	log.Printf("NewInput from %v: [%v]%q", a.Id, len(a.Data), data)

	return nil
}

type NewBugArgs struct {
	Id    int
	Data  string
	Error string
}

func (d *driver) NewBug(a *NewBugArgs, r *int) error {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	if d.Known[a.Error] {
		return nil
	}
	d.Known[a.Error] = true
	d.saveFile(d.KnownDir, a.Error)
	d.saveFile(d.BugDir, a.Data)

	log.Printf("Failed with '%v' on [%v]%s", a.Error, len(a.Data), hex.EncodeToString([]byte(a.Data)))

	return nil
}

type PingArgs struct {
	Id         int
	CorpusSize int
	Execs      uint64
	Coverage   float64
}

type PingRes struct {
	Inputs []string
}

func (d *driver) Ping(a *PingArgs, r *PingRes) error {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	//log.Printf("Ping from %v: corpus=%v cov=%.4f execs=%v", a.Id, a.CorpusSize, a.Coverage*100, a.Execs)
	w := d.Workers[a.Id]
	if w == nil {
		//!!! handle
	}
	w.LastPing = time.Now()
	r.Inputs = w.Pending
	w.Pending = nil
	return nil
}
