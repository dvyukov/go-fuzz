package goast

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

func Fuzz(data []byte) int {
	if false {
		f, err := ioutil.TempFile("", "fuzz.gc")
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
		out, _ := exec.Command("compile", f.Name()).CombinedOutput()
		outs := string(out)
		// "panic" and "fatal error" can be present in normal error messages of compiler.
		// So instead we look for the main compiler source file,
		// which should be present in all crash messages.
		// This does not work either, go-fuzz teaches to massively generate
		// source code that contains "src/cmd/compile/main.go" in error messages. Yikes!
		if strings.Contains(outs, "src/cmd/compile/main.go") {
			panic(outs)
		}
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", data, parser.ParseComments|parser.DeclarationErrors|parser.AllErrors)
	if err != nil {
		return 0
	}
	buf := new(bytes.Buffer)
	printer.Fprint(buf, fset, f)
	fset1 := token.NewFileSet()
	f1, err := parser.ParseFile(fset1, "src.go", buf.Bytes(), parser.ParseComments|parser.DeclarationErrors|parser.AllErrors)
	if err != nil {
		panic(err)
	}
	buf1 := new(bytes.Buffer)
	printer.Fprint(buf1, fset1, f1)
	if !bytes.Equal(buf.Bytes(), buf1.Bytes()) {
		fmt.Printf("source0: %q\n", buf.Bytes())
		fmt.Printf("source1: %q\n", buf1.Bytes())
		panic("source changed")
	}
	return 1
}
