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
	"sync/atomic"
	"syscall"
	"time"
)

var (
	flagCorpus  = flag.String("corpus", "", "dir with input corpus (one file per input)")
	flagWorkdir = flag.String("workdir", "", "dir with persistent work data")
	flagProcs   = flag.Int("procs", runtime.NumCPU(), "parallelism level")
	flagTimeout = flag.Int("timeout", 5000, "test timeout, in ms")
	flagMaster  = flag.String("master", "", "master mode (value is master address)")
	flagSlave   = flag.String("slave", "", "slave mode (value is master address)")
	flagBin     = flag.String("bin", "", "test binary built with go-fuzz-build")
	flagPprof   = flag.String("pprof", "", "serve pprof handlers on that address")
	flagV       = flag.Int("v", 0, "verbosity level")

	shutdown  uint32
	shutdownC = make(chan struct{})
)

func main() {
	buf0 := make([]byte, coverSize)
	buf1 := make([]byte, coverSize)
	for _, v0 := range []byte{0, 1, 2, 3, 4, 127, 128, 129, 255} {
		for _, v1 := range []byte{0, 1, 2, 3, 4, 127, 128, 129, 255} {
			for _, v2 := range []byte{0, 1, 2, 3, 4, 127, 128, 129, 255} {
				for _, v3 := range []byte{0, 1, 2, 3, 4, 127, 128, 129, 255} {
					buf0[0] = v0
					buf0[coverSize-1] = v1
					buf1[0] = v2
					buf1[coverSize-1] = v3
					newCover, newCounter := compareCoverBody(&buf0[0], &buf1[0])
					newCover1, newCounter1 := compareCoverDump(buf0, buf1)
					if newCover != newCover1 || newCounter != newCounter1 {
						println("data:", v0, v1, "/", v2, v3)
						println("res:", newCover1, newCounter1, "/", newCover, newCounter)
						panic("bad")
					}
				}
			}
		}
	}
	//os.Exit(0)

	flag.Parse()
	if *flagMaster != "" && *flagSlave != "" {
		log.Fatalf("both -master and -slave are specified")
	}
	if *flagPprof != "" {
		go http.ListenAndServe(*flagPprof, nil)
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

	runtime.GOMAXPROCS(runtime.NumCPU())
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
		go slaveMain(*flagProcs)
	}

	select {}
}
