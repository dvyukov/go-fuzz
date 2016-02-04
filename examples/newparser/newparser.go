// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package newparser

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"

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
var something = regexp.MustCompile(" constant .* overflows ")

var gcCrash = regexp.MustCompile("\n/tmp/fuzz\\.gc[0-9]+:[0-9]+: internal compiler error: ")
var asanCrash = regexp.MustCompile("\n==[0-9]+==ERROR: AddressSanitizer: ")

func Fuzz(data []byte) int {
	if bigNum.Match(data) || bigNum2.Match(data) {
		return 0
	}
	gotypes(data) // for coverage
	oldGcErr := gc(data, true)
	newGcErr := gc(data, false)

	check := func(gcErr error) bool {
		if gcErr != nil && (gcCrash.MatchString(gcErr.Error()) ||
			strings.Contains(gcErr.Error(), "\nruntime error: ") ||
			strings.HasPrefix(gcErr.Error(), "runtime error: ") ||
			strings.Contains(gcErr.Error(), "%!")) { // bad format string
			if strings.Contains(gcErr.Error(), "internal compiler error: out of fixed registers") {
				// https://github.com/golang/go/issues/11352
				return true
			}
			if strings.Contains(gcErr.Error(), "internal compiler error: treecopy Name") {
				// https://github.com/golang/go/issues/11361
				return true
			}
			if strings.Contains(gcErr.Error(), "internal compiler error: newname nil") {
				// https://github.com/golang/go/issues/11610
				return true
			}
			fmt.Printf("gc result: %v\n", gcErr)
			fmt.Printf("old gc result: %v\n", oldGcErr)
			fmt.Printf("new gc result: %v\n", newGcErr)
			panic("gc compiler crashed")
		}
		return false
	}
	if check(oldGcErr) || check(newGcErr) {
		return 0
	}

	if (oldGcErr == nil) != (newGcErr == nil) {
		fmt.Printf("old gc result: %v\n", oldGcErr)
		fmt.Printf("new gc result: %v\n", newGcErr)
		panic("gcs disagree")
	}

	// too noisy
	if false {
		if oldGcErr != nil && oldGcErr.Error() != newGcErr.Error() {
			fmt.Printf("old gc result: %v\n", oldGcErr)
			fmt.Printf("new gc result: %v\n", newGcErr)
			panic("gcs disagree")
		}
	}
	return 1
}

func gc(data []byte, oldparser bool) error {
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
	var args []string
	if oldparser {
		args = append(args, "-oldparser")
	}
	args = append(args, f.Name())
	out, err := exec.Command("compile", args...).CombinedOutput()
	if err != nil {
		if idx := bytes.Index(out, []byte(", expecting")); idx != -1 {
			out = out[:idx]
		}
		if idx := bytes.IndexByte(out, '\n'); idx != -1 {
			out = out[:idx]
		}
		if idx := bytes.IndexByte(out, ':'); idx != -1 {
			if idx2 := bytes.IndexByte(out[idx+1:], ':'); idx2 != -1 {
				out = out[idx+idx2+2:]
			}
		}
		out = bytes.Replace(out, []byte("syntax error: "), []byte{}, -1)
		return fmt.Errorf("%s\n%s", out, err)
	}
	return nil
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
	prog.Build()
	for _, pkg := range prog.AllPackages() {
		_, err := pkg.WriteTo(ioutil.Discard)
		if err != nil {
			panic(err)
		}
	}
	return
}
