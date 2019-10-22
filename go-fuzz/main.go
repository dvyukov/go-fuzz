// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/tools/go/packages"
)

//go:generate go build github.com/dvyukov/go-fuzz/go-fuzz/vendor/github.com/elazarl/go-bindata-assetfs/go-bindata-assetfs
//go:generate ./go-bindata-assetfs assets/...
//go:generate goimports -w bindata_assetfs.go
//go:generate rm go-bindata-assetfs

var (
	flagWorkdir           = flag.String("workdir", ".", "dir with persistent work data")
	flagProcs             = flag.Int("procs", runtime.NumCPU(), "parallelism level")
	flagTimeout           = flag.Int("timeout", 10, "test timeout, in seconds")
	flagMinimize          = flag.Duration("minimize", 1*time.Minute, "time limit for input minimization")
	flagCoordinator       = flag.String("coordinator", "", "coordinator mode (value is coordinator address)")
	flagWorker            = flag.String("worker", "", "worker mode (value is coordinator address)")
	flagConnectionTimeout = flag.Duration("connectiontimeout", 1*time.Minute, "time limit for worker to try to connect coordinator")
	flagBin               = flag.String("bin", "", "test binary built with go-fuzz-build")
	flagFunc              = flag.String("func", "", "function to fuzz")
	flagDumpCover         = flag.Bool("dumpcover", false, "dump coverage profile into workdir")
	flagDup               = flag.Bool("dup", false, "collect duplicate crashers")
	flagTestOutput        = flag.Bool("testoutput", false, "print test binary output to stdout (for debugging only)")
	flagCoverCounters     = flag.Bool("covercounters", true, "use coverage hit counters")
	flagSonar             = flag.Bool("sonar", true, "use sonar hints")
	flagV                 = flag.Int("v", 0, "verbosity level")
	flagHTTP              = flag.String("http", "", "HTTP server listen address (coordinator mode only)")

	shutdown        uint32
	shutdownC       = make(chan struct{})
	shutdownCleanup []func()
)

func main() {
	flag.Parse()
	if *flagCoordinator != "" && *flagWorker != "" {
		log.Fatalf("both -coordinator and -worker are specified")
	}
	if *flagHTTP != "" && *flagWorker != "" {
		log.Fatalf("both -http and -worker are specified")
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

	*flagWorkdir = expandHomeDir(*flagWorkdir)
	*flagBin = expandHomeDir(*flagBin)

	if *flagCoordinator != "" || *flagWorker == "" {
		if *flagWorkdir == "" {
			log.Fatalf("-workdir is not set")
		}
		if *flagCoordinator == "" {
			*flagCoordinator = "localhost:0"
		}
		ln, err := net.Listen("tcp", *flagCoordinator)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		if *flagCoordinator == "localhost:0" && *flagWorker == "" {
			*flagWorker = ln.Addr().String()
		}
		go coordinatorMain(ln)
	}

	if *flagWorker != "" {
		if *flagBin == "" {
			// Try the default. Best effort only.
			var bin string
			cfg := new(packages.Config)
			// Note that we do not set GO111MODULE here in order to respect any GO111MODULE 
			// setting by the user as we are finding dependencies. See modules support 
			// comments in go-fuzz-build/main.go for more details.
			cfg.Env = os.Environ()
			pkgs, err := packages.Load(cfg, ".")
			if err == nil && len(pkgs) == 1 {
				bin = pkgs[0].Name + "-fuzz.zip"
				_, err := os.Stat(bin)
				if err != nil {
					bin = ""
				}
			}
			if bin == "" {
				log.Fatalf("-bin is not set")
			}
			*flagBin = bin
		}
		go workerMain()
	}

	select {}
}

// expandHomeDir expands the tilde sign and replaces it
// with current users home directory and returns it.
func expandHomeDir(path string) string {
	if len(path) > 2 && path[:2] == "~/" {
		usr, _ := user.Current()
		path = filepath.Join(usr.HomeDir, path[2:])
	}
	return path
}
