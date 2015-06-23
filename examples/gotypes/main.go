package gotypes

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	_ "golang.org/x/tools/go/gcimporter"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
)

// https://github.com/golang/go/issues/11327
var bigNum = regexp.MustCompile("(\\.[0-9]*)|([0-9]+)[eE]\\-?[0-9]{3,}")

// https://github.com/golang/go/issues/11274
var formatBug1 = regexp.MustCompile("\\*/[ \t\n\r\f\v]*;")
var formatBug2 = regexp.MustCompile(";[ \t\n\r\f\v]*/\\*")

var gcCrash = regexp.MustCompile("\n/tmp/fuzz\\.gc[0-9]+:[0-9]+: internal compiler error: ")

func Fuzz(data []byte) int {
	if bigNum.Match(data) {
		return 0
	}
	goErr := gotypes(data)
	gcErr := gc(data)
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "line number out of range") {
		// https://github.com/golang/go/issues/11329
		return 0
	}
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "stupid shift:") {
		// https://github.com/golang/go/issues/11328
		return 0
	}
	if gcErr == nil && goErr != nil && strings.Contains(goErr.Error(), "untyped float constant") {
		// https://github.com/golang/go/issues/11350
		return 0
	}
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "(type float64) to type string") {
		// https://github.com/golang/go/issues/11353
		return 0
	}
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "(type complex128) to type string") {
		// https://github.com/golang/go/issues/11357
		return 0
	}
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "overflow in int -> string") {
		// https://github.com/golang/go/issues/11330
		return 0
	}
	if gcErr == nil && goErr != nil && strings.Contains(goErr.Error(), "illegal character U+") {
		// https://github.com/golang/go/issues/11359
		return 0
	}
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "larger than address space") {
		// Gc is more picky at rejecting huge objects.
		return 0
	}
	if goErr == nil && gcErr != nil && strings.Contains(gcErr.Error(), "non-canonical import path") {
		return 0
	}
	// go-fuzz is too smart so it can generate a program that contains "internal compiler error" in an error message :)
	if gcErr != nil && gcCrash.MatchString(gcErr.Error()) {
		if strings.Contains(gcErr.Error(), "internal compiler error: out of fixed registers") {
			// https://github.com/golang/go/issues/11352
			return 0
		}
		if strings.Contains(gcErr.Error(), "internal compiler error: naddr: bad HMUL") {
			// https://github.com/golang/go/issues/11358
			return 0
		}
		if strings.Contains(gcErr.Error(), "internal compiler error: treecopy Name") {
			// https://github.com/golang/go/issues/11361
			return 0
		}
		fmt.Printf("gc result: %v\n", gcErr)
		panic("gc compiler crashed")
	}
	if (goErr == nil) != (gcErr == nil) {
		fmt.Printf("go/types result: %v\n", goErr)
		fmt.Printf("gc result: %v\n", gcErr)
		panic("gc and go/types disagree")
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
	err = gotypes(data1)
	if err != nil {
		fmt.Printf("new: %q\n", data1)
		fmt.Printf("err: %v\n", err)
		panic("program become invalid after gofmt")
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
	_, err = types.Check("pkg", fset, []*ast.File{f})
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
