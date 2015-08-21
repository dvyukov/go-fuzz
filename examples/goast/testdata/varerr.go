// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// errorcheck

// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Verify that a couple of illegal variable declarations are caught by the compiler.
// Does not compile.

package main

func main() {
	_ = asdf	// ERROR "undefined.*asdf"

	new = 1	// ERROR "use of builtin new not in function call|invalid left hand side"
}

