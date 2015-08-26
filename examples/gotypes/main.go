// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package gotypes

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// https://github.com/golang/go/issues/11327
var bigNum = regexp.MustCompile("(\\.[0-9]*)|([0-9]+)[eE]\\-?\\+?[0-9]{3,}")
var bigNum2 = regexp.MustCompile("[0-9]+[pP][0-9]{3,}") // see issue 11364

// https://github.com/golang/go/issues/11274
var formatBug1 = regexp.MustCompile("\\*/[ \t\n\r\f\v]*;")
var formatBug2 = regexp.MustCompile(";[ \t\n\r\f\v]*/\\*")

var issue11590 = regexp.MustCompile(": cannot convert .* \\(untyped int constant .*\\) to complex")
var issue11590_2 = regexp.MustCompile(": [0-9]+ (untyped int constant) overflows complex")
var issue11370 = regexp.MustCompile("\\\"[ \t\n\r\f\v]*\\[")

var fpRounding = regexp.MustCompile(" \\(untyped float constant .*\\) truncated to ")

var gcCrash = regexp.MustCompile("\n/tmp/fuzz\\.gc[0-9]+:[0-9]+: internal compiler error: ")
var asanCrash = regexp.MustCompile("\n==[0-9]+==ERROR: AddressSanitizer: ")

func Fuzz(data []byte) int {
	if bigNum.Match(data) || bigNum2.Match(data) {
		return 0
	}
	goErr := gotypes(data)
	gcErr := gc(data)
	gccgoErr := gccgo(data)
	if goErr == nil && gcErr != nil {
		if strings.Contains(gcErr.Error(), "line number out of range") {
			// https://github.com/golang/go/issues/11329
			return 0
		}
		if strings.Contains(gcErr.Error(), "overflow in int -> string") {
			// https://github.com/golang/go/issues/11330
			return 0
		}
		if strings.Contains(gcErr.Error(), "larger than address space") {
			// Gc is more picky at rejecting huge objects.
			return 0
		}
		if strings.Contains(gcErr.Error(), "non-canonical import path") {
			return 0
		}
		if strings.Contains(gcErr.Error(), "constant shift overflow") {
			// ???
			return 0
		}
	}

	if gcErr == nil && goErr != nil {
		if strings.Contains(goErr.Error(), "illegal character U+") {
			// https://github.com/golang/go/issues/11359
			return 0
		}
		if issue11590.MatchString(goErr.Error()) || issue11590_2.MatchString(goErr.Error()) {
			// https://github.com/golang/go/issues/11590
			return 0
		}
		if issue11370.MatchString(goErr.Error()) {
			return 0
		}
	}

	if gccgoErr == nil && goErr != nil {
		if strings.Contains(goErr.Error(), "invalid operation: stupid shift count") {
			// https://github.com/golang/go/issues/11524
			return 0
		}
		if (bytes.Contains(data, []byte("//line")) || bytes.Contains(data, []byte("/*"))) &&
			(strings.Contains(goErr.Error(), "illegal UTF-8 encoding") ||
				strings.Contains(goErr.Error(), "illegal character NUL")) {
			// https://github.com/golang/go/issues/11527
			return 0
		}
		if fpRounding.MatchString(goErr.Error()) {
			// gccgo has different rounding
			return 0
		}
		if strings.Contains(goErr.Error(), "operator | not defined for") {
			// https://github.com/golang/go/issues/11566
			return 0
		}
		if strings.Contains(goErr.Error(), "illegal byte order mark") {
			// on "package\rG\n//line \ufeff:1" input, not filed.
			return 0
		}
	}

	if goErr == nil && gccgoErr != nil {
		if strings.Contains(gccgoErr.Error(), "error: integer constant overflow") {
			// https://github.com/golang/go/issues/11525
			return 0
		}
		if bytes.Contains(data, []byte("0i")) &&
			(strings.Contains(gccgoErr.Error(), "incompatible types in binary expression") ||
				strings.Contains(gccgoErr.Error(), "initialization expression has wrong type")) {
			// https://github.com/golang/go/issues/11564
			// https://github.com/golang/go/issues/11563
			return 0
		}
	}

	if gcErr != nil && goErr != nil && gccgoErr == nil && strings.Contains(gcErr.Error(), "declared and not used") && strings.Contains(goErr.Error(), "declared but not used") {
		// https://github.com/golang/go/issues/12317
		return 0
	}

	// go-fuzz is too smart so it can generate a program that contains "internal compiler error" in an error message :)
	if gcErr != nil && (gcCrash.MatchString(gcErr.Error()) ||
		strings.Contains(gcErr.Error(), "\nruntime error: ") ||
		strings.HasPrefix(gcErr.Error(), "runtime error: ") ||
		strings.Contains(gcErr.Error(), "%!")) { // bad format string
		if strings.Contains(gcErr.Error(), "internal compiler error: out of fixed registers") {
			// https://github.com/golang/go/issues/11352
			return 0
		}
		if strings.Contains(gcErr.Error(), "internal compiler error: treecopy Name") {
			// https://github.com/golang/go/issues/11361
			return 0
		}
		if strings.Contains(gcErr.Error(), "internal compiler error: newname nil") {
			// https://github.com/golang/go/issues/11610
			return 0
		}
		fmt.Printf("gc result: %v\n", gcErr)
		panic("gc compiler crashed")
	}

	const gccgoCrash = "go1: internal compiler error:"
	if gccgoErr != nil && (strings.HasPrefix(gccgoErr.Error(), gccgoCrash) || strings.Contains(gccgoErr.Error(), "\n"+gccgoCrash)) {
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in define, at go/gofrontend/gogo.h") {
			// https://github.com/golang/go/issues/12316
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in set_type, at go/gofrontend/expressions.cc") {
			// https://github.com/golang/go/issues/11537
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in global_variable_set_init, at go/go-gcc.cc") {
			// https://github.com/golang/go/issues/11541
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in record_var_depends_on, at go/gofrontend/gogo.h") {
			// https://github.com/golang/go/issues/11543
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in Builtin_call_expression, at go/gofrontend/expressions.cc") {
			// https://github.com/golang/go/issues/11544
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in check_bounds, at go/gofrontend/expressions.cc") {
			// https://github.com/golang/go/issues/11545
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in backend_numeric_constant_expression, at go/gofrontend/expressions.cc") {
			// https://github.com/golang/go/issues/11548
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in type_size, at go/go-gcc.cc") {
			// https://github.com/golang/go/issues/11554
			// https://github.com/golang/go/issues/11555
			// https://github.com/golang/go/issues/11556
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in do_flatten, at go/gofrontend/expressions.cc") {
			// https://github.com/golang/go/issues/12319
			// https://github.com/golang/go/issues/12320
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in do_export, at go/gofrontend/types.cc") {
			// https://github.com/golang/go/issues/12321
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in start_function, at go/gofrontend/gogo.cc") {
			// https://github.com/golang/go/issues/12324
			return 0
		}
		if strings.Contains(gccgoErr.Error(), "go1: internal compiler error: in do_get_backend, at go/gofrontend/expressions.cc") {
			// https://github.com/golang/go/issues/12325
			return 0
		}
		fmt.Printf("gccgo result: %v\n", gccgoErr)
		panic("gccgo compiler crashed")
	}

	if gccgoErr != nil && asanCrash.MatchString(gccgoErr.Error()) {
		if strings.Contains(gccgoErr.Error(), " in Lex::skip_cpp_comment() ../../gcc/go/gofrontend/lex.cc") {
			// https://github.com/golang/go/issues/11577
			return 0
		}
		fmt.Printf("gccgo result: %v\n", gccgoErr)
		panic("gccgo compiler crashed")
	}

	if gcErr == nil && goErr == nil && gccgoErr != nil && strings.Contains(gccgoErr.Error(), "0x124a4") {
		// https://github.com/golang/go/issues/12322
		return 0
	}

	if (goErr == nil) != (gcErr == nil) || (goErr == nil) != (gccgoErr == nil) {
		fmt.Printf("go/types result: %v\n", goErr)
		fmt.Printf("gc result: %v\n", gcErr)
		fmt.Printf("gccgo result: %v\n", gccgoErr)
		panic("gc, gccgo and go/types disagree")
	}
	if goErr != nil {
		return 0

	}
	if formatBug1.Match(data) || formatBug2.Match(data) {
		return 1
	}
	// https://github.com/golang/go/issues/11274
	data = bytes.Replace(data, []byte{'\r'}, []byte{' '}, -1)
	data1, err := format.Source(data)
	if err != nil {
		panic(err)
	}
	if false {
		err = gotypes(data1)
		if err != nil {
			fmt.Printf("new: %q\n", data1)
			fmt.Printf("err: %v\n", err)
			panic("program become invalid after gofmt")
		}
	}
	return 1
}

func gotypes(data []byte) (err error) {
	fset := token.NewFileSet()
	var f *ast.File
	f, err = parser.ParseFile(fset, "src.go", data, parser.ParseComments|parser.DeclarationErrors|parser.AllErrors)
	if err != nil {
		return
	}
	// provide error handler
	// initialize maps in config
	conf := &types.Config{
		Error:    func(err error) {},
		Sizes:    &types.StdSizes{8, 8},
		Importer: importer.For("gc", nil),
	}
	_, err = conf.Check("pkg", fset, []*ast.File{f}, nil)
	if err != nil {
		return
	}
	prog := ssa.NewProgram(fset, ssa.BuildSerially|ssa.SanityCheckFunctions|ssa.GlobalDebug)
	prog.BuildAll()
	for _, pkg := range prog.AllPackages() {
		_, err := pkg.WriteTo(ioutil.Discard)
		if err != nil {
			panic(err)
		}
	}
	return
}

func gc(data []byte) error {
	f, err := ioutil.TempFile("", "fuzz.gc")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	f.Close()
	out, err := exec.Command("compile", f.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s\n%s", out, err)
	}
	return nil
}

func gccgo(data []byte) error {
	cmd := exec.Command("gccgo", "-c", "-x", "go", "-O3", "-o", "/dev/null", "-")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s\n%s", out, err)
	}
	return nil
}
