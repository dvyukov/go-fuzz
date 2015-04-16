package main

import (
	"flag"
	"log"
	"runtime"
	"net"
)

var (
	flagCorpus       = flag.String("corpus", "", "dir with input corpus (one file per input)")
	flagWorkdir      = flag.String("workdir", "", "dir with persistent work data")
	flagProcs        = flag.Int("procs", runtime.NumCPU(), "parallelism level")
	flagMaster       = flag.String("master", "", "master mode (value is master address)")
	flagSlave       = flag.String("slave", "", "slave mode (value is master address)")
	flagBin = flag.String("bin", "", "test binary built with go-fuzz-build")
	flagV            = flag.Bool("v", false, "verbose mode")
)

func main() {
	flag.Parse()
	if *flagCorpus == "" {
		log.Fatalf("-corpus is not set")
	}
	if *flagWorkdir == "" {
		log.Fatalf("-workdir is not set")
	}
	if *flagBin == "" {
		log.Fatalf("-bin is not set")
	}
	if *flagMaster != "" && *flagSlave != "" {
		log.Fatalf("both -master and -slave are specified")
	}

	if *flagMaster != "" || *flagSlave == "" {
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
		go master(ln)
	}		

	if *flagSlave != "" {
		for i := 0; i < *flagProcs; i++ {
			go slave()
		}
	}

	select {}
}
