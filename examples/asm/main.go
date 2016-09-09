// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// This file needs to be copied to GOROOT/src/cmd/asm/fuzz/fuzz.go,
// otherwise it will fail to import internal packages.
// Also apply the following patch to std lib:

/*
diff --git a/src/cmd/asm/internal/asm/parse.go b/src/cmd/asm/internal/asm/parse.go
index c07e6f8..d39cb47 100644
--- a/src/cmd/asm/internal/asm/parse.go
+++ b/src/cmd/asm/internal/asm/parse.go
@@ -8,7 +8,7 @@ package asm

 import (
        "fmt"
-       "log"
        "os"
        "strconv"
        "text/scanner"
@@ -73,7 +73,8 @@ func (p *Parser) errorf(format string, args ...interface{}) {
        fmt.Fprintf(os.Stderr, format, args...)
        p.errorCount++
        if p.errorCount > 10 {
-               log.Fatal("too many errors")
+               panic("os.Exit")
        }
 }

diff --git a/src/cmd/asm/internal/lex/input.go b/src/cmd/asm/internal/lex/input.go
index 7e495b8..45e9b8d 100644
--- a/src/cmd/asm/internal/lex/input.go
+++ b/src/cmd/asm/internal/lex/input.go
@@ -64,8 +64,9 @@ func predefine(defines flags.MultiFlag) map[string]*Macro {
 }

 func (in *Input) Error(args ...interface{}) {
-       fmt.Fprintf(os.Stderr, "%s:%d: %s", in.File(), in.Line(), fmt.Sprintln(args...))
-       os.Exit(1)
+       panic("os.Exit")
 }
*/

package asm

import (
	"bytes"
	"cmd/asm/internal/arch"
	"cmd/asm/internal/asm"
	"cmd/asm/internal/lex"
	"cmd/internal/obj"
	"io/ioutil"
	"os"
)

func Fuzz(data []byte) int {
	f, err := ioutil.TempFile("", "fuzz.asm")
	if err != nil {
		return 0
	}
	defer os.Remove(f.Name())
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return 0
	}
	f.Close()

	defer func() {
		if x := recover(); x != nil {
			if str, ok := x.(string); ok && str == "os.Exit" {
				return
			}
			panic(x)
		}
	}()

	const GOARCH = "amd64"
	architecture := arch.Set(GOARCH)
	fd := new(bytes.Buffer)
	ctxt := obj.Linknew(architecture.LinkArch)
	// Try to vary these and other flags:
	// ctxt.Flag_dynlink
	// ctxt.Flag_shared
	ctxt.Bso = obj.Binitw(new(bytes.Buffer))
	defer ctxt.Bso.Flush()
	ctxt.Diag = func(format string, v ...interface{}) { panic("os.Exit") }
	output := obj.Binitw(fd)
	lexer := lex.NewLexer(f.Name(), ctxt)
	parser := asm.NewParser(ctxt, architecture, lexer)
	pList := obj.Linknewplist(ctxt)
	var ok bool
	pList.Firstpc, ok = parser.Parse()
	if !ok {
		return 0
	}
	obj.Writeobjdirect(ctxt, output)
	output.Flush()
	return 1
}
