// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package flag

import (
	"flag"
)

func Fuzz(data []byte) int {
	fs := flag.NewFlagSet("name", flag.ContinueOnError)
	fs.Bool("A", false, "-")
	fs.Duration("B", 0, "-")
	fs.Float64("C", 0, "-")
	fs.Int("D", 0, "-")
	fs.Int64("E", 0, "-")
	fs.Uint("F", 0, "-")
	fs.Uint64("G", 0, "-")
	args := []string{string(data[:len(data)/2]), string(data[len(data)/2:])}
	if fs.Parse(args) != nil {
		return 0
	}
	for i := 0; i < fs.NArg(); i++ {
		_ = fs.Arg(i)
	}
	return 1
}
