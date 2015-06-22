package gotypes

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"

	"golang.org/x/tools/go/types"
	_ "golang.org/x/tools/go/gcimporter"
)

func Fuzz(data []byte) int {
	goErr := gotypes(data)
	gcErr := gc(data)
	if (goErr == nil) != (gcErr == nil) {
		fmt.Printf("go/types result: %v\n", goErr)
		fmt.Printf("gc result: %v\n", gcErr)
		panic("gc and go/types disagree")
	}
	if goErr != nil {
		return 0
	}
	return 1
}

func gotypes(data []byte) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", data, parser.ParseComments|parser.DeclarationErrors|parser.AllErrors)
	if err != nil {
		return err
	}
	_, err = types.Check("pkg", fset, []*ast.File{f})
	if err != nil {
		return err
	}
	return nil
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
