package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/stephens2424/writerset"
)

var (
	flagWorkdir       = flag.String("workdir", "", "dir with persistent work data")
	flagProcs         = flag.Int("procs", runtime.NumCPU(), "parallelism level")
	flagTimeout       = flag.Int("timeout", 10, "test timeout, in seconds")
	flagMinimize      = flag.Duration("minimize", 1*time.Minute, "time limit for input minimization")
	flagMaster        = flag.String("master", "", "master mode (value is master address)")
	flagSlave         = flag.String("slave", "", "slave mode (value is master address)")
	flagBin           = flag.String("bin", "", "test binary built with go-fuzz-build")
	flagPprof         = flag.String("pprof", "", "serve pprof handlers on that address")
	flagDumpCover     = flag.Bool("dumpcover", false, "dump coverage profile into workdir")
	flagTestOutput    = flag.Bool("testoutput", false, "print test binary output to stdout (for debugging only)")
	flagCoverCounters = flag.Bool("covercounters", true, "use coverage hit counters")
	flagSonar         = flag.Bool("sonar", true, "use sonar hints")
	flagV             = flag.Int("v", 0, "verbosity level")
	flagMasterStats   = flag.String("masterstats", "", "serve master stats handlers on that address")

	shutdown        uint32
	shutdownC       = make(chan struct{})
	shutdownCleanup []func()

	writerSet = writerset.New()
)

func main() {
	flag.Parse()
	if *flagMaster != "" && *flagSlave != "" {
		log.Fatalf("both -master and -slave are specified")
	}
	if *flagMasterStats != "" && *flagSlave != "" {
		log.Fatalf("both -masterstats and -slave are specified")
	}
	if *flagPprof != "" {
		go http.ListenAndServe(*flagPprof, nil)
	} else {
		runtime.MemProfileRate = 0
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT)
		<-c
		atomic.StoreUint32(&shutdown, 1)
		close(shutdownC)
		log.Printf("shutting down...")
		time.Sleep(2 * time.Second)
		for _, f := range shutdownCleanup {
			f()
		}
		os.Exit(0)
	}()

	runtime.GOMAXPROCS(min(*flagProcs, runtime.NumCPU()))
	debug.SetGCPercent(50) // most memory is in large binary blobs
	lowerProcessPrio()

	if *flagMaster != "" || *flagSlave == "" {
		if *flagWorkdir == "" {
			log.Fatalf("-workdir is not set")
		}
		if *flagMaster == "" {
			*flagMaster = "localhost:0"
		}
		ln, err := net.Listen("tcp", *flagMaster)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		if *flagMaster == "localhost:0" && *flagSlave == "" {
			*flagSlave = ln.Addr().String()
		}
		go masterMain(ln)
	}

	if *flagSlave != "" {
		if *flagBin == "" {
			log.Fatalf("-bin is not set")
		}
		go slaveMain()
	}

	if *flagMasterStats != "" {
		evMux := http.NewServeMux()
		evMux.HandleFunc("/eventsource", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			<-writerSet.Add(w)
		})
		evMux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, statsPage)
		})

		go http.ListenAndServe(*flagMasterStats, evMux)
	}

	select {}
}

func broadcastStats(stats masterStats) {
	b, err := json.Marshal(stats)
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(writerSet, "event: ping\ndata: %s\n\n", string(b))
	writerSet.Flush()
}
