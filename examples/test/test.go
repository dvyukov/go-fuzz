// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package test

import (
	"bytes"
	"runtime"

	// Test vendoring support.
	vendored_foo "non.existent.com/foo"
)

func init() {
	vendored_foo.Foo()
	// Test that background goroutines don't break sonar.
	// https://github.com/dvyukov/go-fuzz/issues/145
	// Sonar code is racy (see the issue), but the test don't crash
	// unless runtime.GOMAXPROCS is uncommented below.
	go func() {
		x := 0
		s := "foobarbazqux"
		for i := 0; ; i++ {
			runtime.Gosched()
			if i == x {
				s = "foobarbazquz"
				x -= 1
			}
			if s == "foo" {
				x -= 1
			}
		}
	}()
}

func Fuzz(data []byte) int {
	// runtime.GOMAXPROCS(runtime.NumCPU())
	if len(data) == 1 {
		if data[0] == '!' || data[0] == '#' {
			panic("bingo 0")
		}
		if data[0] == '0' || data[0] == '9' {
			for {
				c := make(chan bool)
				close(c)
			}
		}
		if data[0] == 'a' || data[0] == 'z' {
			data := make([]byte, 128<<30-1)
			_ = data
		}
		if data[0] == 'b' {
			// new coverage
		}
		if data[0] == 'c' {
			// new coverage
		}
	}

	// Test for crash minimization.
	if bytes.IndexByte(data, 'x') != -1 {
		if bytes.IndexByte(data, 'y') != -1 {
			panic("xy")
		}
	}

	// Test for input minimization.
	if bytes.Index(data, []byte("input ")) != -1 {
		if bytes.Index(data, []byte("minimization ")) != -1 {
			if bytes.Index(data, []byte("test")) != -1 {
			}
		}
	}

	if len(data) >= 14 && bytes.HasPrefix(data, []byte("0123456789")) {
		x := int(data[10]) + int(data[11])<<8 + int(data[12])<<16 + int(data[13])<<24
		if x == 0 || x == -1 {
			panic("bingo 1")
		}
		if x == 255 || x == 256 {
			for {
				c := make(chan bool)
				close(c)
			}
		}
		if x == 1<<16-1 || x == 1<<16 {
			data := make([]byte, 128<<30-1)
			_ = data
		}
		if x == '1' {
			// new coverage
		}
		if x == '2' {
			// new coverage
		}
	}
	return 0
}

// Compilation tests, go-fuzz-build previously failed on these code patterns.

// Test for issue #35.
const X = 1 << 129

func foo(x float64) bool {
	return x < X
}

func test1() bool {
	var x uint64
	var y uint
	return x == 1<<y
}

func test11() bool {
	var x uint64
	var y uint
	return x < (1<<uint64(y))-1
}

func Pow(x, y float64) float64 {
	switch {
	case x == -1:
		return 1
	case (Abs(x) < 1) == IsInf(y, 1):
		return 0
	default:
		return 1
	}
}

func Abs(x float64) float64 {
	return x
}

func IsInf(x float64, v int) bool {
	return x != 0
}

func test2(p *int) bool {
	return p == nil
}

type ChanDir int

const (
	SEND ChanDir = 1 << iota
	RECV
)

func test3(x ChanDir) bool {
	return x == SEND|RECV
}

type MyBool bool

func test4(x, y MyBool) MyBool {
	if x && y {
		return true
	}
	if true && y {
		return true
	}
	if x && true {
		return true
	}
	return false
}

func bla() error {
	return nil
}

func test5() bool {
	return nil == bla()
}
