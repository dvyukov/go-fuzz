package goast

import (
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
		out, _ := exec.Command("6g", f.Name()).CombinedOutput()
		outs := string(out)
		if strings.Contains(outs, "fatal error") || strings.Contains(outs, "panic:") {
			panic(outs)
		}
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", data, parser.ParseComments|parser.DeclarationErrors|parser.AllErrors)
	if err != nil {
		return 0
	}
	printer.Fprint(ioutil.Discard, fset, f)
	return 1
}
