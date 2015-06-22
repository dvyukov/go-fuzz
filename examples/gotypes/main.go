package gotypes

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	_ "golang.org/x/tools/go/gcimporter"
	"golang.org/x/tools/go/types"
)

func Fuzz(data []byte) int {
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

func gotypes(data []byte) (err error) {
	defer func() {
		x := recover()
		if x != nil {
			if str, ok := x.(string); ok && strings.Contains(str, "not an Int") {
				// https://github.com/golang/go/issues/11325
				err = errors.New(str)
				return
			}
			panic(x)
		}
	}()
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
