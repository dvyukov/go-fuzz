// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// errorcheck

// Verify that the Go compiler will not
// die after running into an undefined
// type in the argument list for a
// function.
// Does not compile.

package main

func mine(int b) int {	// ERROR "undefined.*b"
	return b + 2	// ERROR "undefined.*b"
}

func main() {
	mine()		// GCCGO_ERROR "not enough arguments"
	c = mine()	// ERROR "undefined.*c|not enough arguments"
}
