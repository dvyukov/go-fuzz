// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package gen

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var (
	flagOut = flag.String("out", "", "output dir")
	flagN   = flag.Int("n", 1000, "number of inputs to generate")
	seq     = 0
)

func init() {
	flag.Parse()
	if *flagOut == "" {
		fmt.Fprintf(os.Stderr, "output directory is not set\n")
		os.Exit(1)
	}
	if err := os.MkdirAll(*flagOut, 0760); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir failed: %v\n", err)
		os.Exit(1)
	}
	rand.Seed(time.Now().UnixNano())
}

func Rand(n int) int {
	return rand.Intn(n)
}

func Emit(data, hint []byte, valid bool) {
	f, err := os.Create(filepath.Join(*flagOut, fmt.Sprintf("%d", seq)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	n, err := f.Write(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write to file: %v\n", err)
		os.Exit(1)
	} else if n != len(data) {
		fmt.Fprint(os.Stderr, "failed to write data to file\n")
		os.Exit(1)
	}

	if seq++; seq == *flagN {
		os.Exit(0)
	}
}
