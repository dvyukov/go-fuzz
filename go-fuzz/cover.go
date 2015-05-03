package main

import (
	"fmt"
	"log"
	"os"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

func compareCover(base, cur []byte) (bool, bool) {
	if len(base) != CoverSize || len(cur) != CoverSize {
		log.Fatalf("bad cover table size (%v, %v)", len(base), len(cur))
	}
	newCover, newCounter := compareCoverBody(&base[0], &cur[0])
	if false {
		newCover1, newCounter1 := compareCoverDump(base, cur)
		if newCover != newCover1 || newCounter != newCounter1 {
			panic("bad")
		}
	}
	return newCover, newCounter
}

func compareCoverDump(base, cur []byte) (bool, bool) {
	cnt := false
	for i, v := range base {
		x := cur[i]
		if v == 0 && x != 0 {
			return true, true
		}
		if x > v {
			cnt = true
		}
	}
	return false, cnt
}

func compareCoverBody(base, cur *byte) (bool, bool) // in compare.s

func updateMaxCover(base, cur []byte) int {
	if len(base) != CoverSize || len(cur) != CoverSize {
		log.Fatalf("bad cover table size (%v, %v)", len(base), len(cur))
	}
	cnt := 0
	for i, x := range cur {
		// Quantize the counters.
		// Otherwise we get too inflated corpus.
		if x == 0 {
			x = 0
		} else if x <= 1 {
			x = 1
		} else if x <= 2 {
			x = 2
		} else if x <= 4 {
			x = 4
		} else if x <= 8 {
			x = 8
		} else if x <= 16 {
			x = 16
		} else if x <= 32 {
			x = 32
		} else if x <= 64 {
			x = 64
		} else {
			x = 255
		}
		v := base[i]
		if v != 0 || x > 0 {
			cnt++
		}
		if v < x {
			base[i] = x
		}
	}
	return cnt
}

func dumpCover(outf string, blocks map[int][]CoverBlock, cover []byte) {
	// Exclude files that have no coverage at all.
	files := make(map[string]bool)
	for i, v := range cover {
		if v == 0 {
			continue
		}
		for _, b := range blocks[i] {
			files[b.File] = true
		}
	}

	out, err := os.Create(outf)
	if err != nil {
		log.Fatalf("failed to create coverage file: %v", err)
	}
	defer out.Close()
	const showCounters = false
	if showCounters {
		fmt.Fprintf(out, "mode: count\n")
	} else {
		fmt.Fprintf(out, "mode: set\n")
	}
	for i, v := range cover {
		for _, b := range blocks[i] {
			if !files[b.File] {
				continue
			}
			if !showCounters && v != 0 {
				v = 1
			}
			fmt.Fprintf(out, "%s:%v.%v,%v.%v %v %v\n",
				b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, v)
		}
	}
}

func dumpSonar(outf string, sites []SonarSite) {
	out, err := os.Create(outf)
	if err != nil {
		log.Fatalf("failed to create coverage file: %v", err)
	}
	defer out.Close()
	fmt.Fprintf(out, "mode: set\n")
	for _, s := range sites {
		cnt := 1
		stmt := 1
		if s.taken == 0 {
			stmt = 0
			cnt = 0
		} else if s.taken == 3 {
			cnt = 100
		}
		fmt.Fprintf(out, "%v %v %v\n", s.loc, stmt, cnt)
	}
}
