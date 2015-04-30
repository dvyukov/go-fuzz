package main

import (
	"flag"
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
)

var (
	flagCorpus   = flag.String("corpus", "", "dir with input corpus (one file per input)")
	flagWorkdir  = flag.String("workdir", "", "dir with persistent work data")
	flagProcs    = flag.Int("procs", runtime.NumCPU(), "parallelism level")
	flagTimeout  = flag.Int("timeout", 10, "test timeout, in seconds")
	flagMinimize = flag.Duration("minimize", 10*time.Second, "time limit for input minimization")
	flagMaster   = flag.String("master", "", "master mode (value is master address)")
	flagSlave    = flag.String("slave", "", "slave mode (value is master address)")
	flagBin      = flag.String("bin", "", "test binary built with go-fuzz-build")
	flagPprof    = flag.String("pprof", "", "serve pprof handlers on that address")
	flagV        = flag.Int("v", 0, "verbosity level")

	shutdown  uint32
	shutdownC = make(chan struct{})
)

func main() {
	flag.Parse()
	if *flagMaster != "" && *flagSlave != "" {
		log.Fatalf("both -master and -slave are specified")
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
		os.Exit(0)
	}()

	runtime.GOMAXPROCS(min(*flagProcs, runtime.NumCPU()))
	debug.SetGCPercent(50) // most memory is in large binary blobs
	syscall.Setpriority(syscall.PRIO_PROCESS, 0, 19)

	if *flagMaster != "" || *flagSlave == "" {
		if *flagCorpus == "" {
			log.Fatalf("-corpus is not set")
		}
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

	select {}
}
