package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	flagOut        = flag.String("o", "", "output file")
	flagFunc       = flag.String("func", "Fuzz", "entry function")
	flagWork       = flag.Bool("work", false, "don't remove working directory")
	flagInstrument = flag.String("instrument", "", "instrument a single file (for debugging)")

	workdir string
)

const (
	mainPkg = "go-fuzz-main"
)

// Copies the package with all dependent packages into a temp dir,
// instruments Go source files there and builds setting GOROOT to the temp dir.
func main() {
	flag.Parse()
	if *flagInstrument != "" {
		f, err := ioutil.TempFile("", "go-fuzz-instrument-")
		if err != nil {
			failf("failed to create temp file: %v", err)
		}
		f.Close()
		instrument("pkg", "pkg/file.go", *flagInstrument, f.Name(), make(map[string]bool), make(map[string][]Block), false)
		data, err := ioutil.ReadFile(f.Name())
		if err != nil {
			failf("failed to read temp file: %v", err)
		}
		fmt.Println(string(data))
		os.Exit(0)
	}
	if len(flag.Args()) != 1 || len(flag.Arg(0)) == 0 {
		failf("usage: go-fuzz-build pkg")
	}
	if os.Getenv("GOROOT") == "" {
		// Figure out GOROOT from go command location.
		out, err := exec.Command("which", "go").CombinedOutput()
		if err != nil || len(out) == 0 {
			failf("GOROOT is not set and failed to locate go command: 'which go' returned '%s' (%v)", out, err)
		}
		os.Setenv("GOROOT", filepath.Dir(filepath.Dir(string(out))))
	}
	pkg := flag.Arg(0)
	if pkg[0] == '.' {
		failf("relative import paths are not supported, please specify full package name")
	}

	// To produce error messages (this is much faster and gives correct line numbers).
	testNormalBuild(pkg)

	deps := make(map[string]bool)
	for _, p := range goListList(pkg, "Deps") {
		deps[p] = true
	}
	deps[pkg] = true
	// These packages are used by go-fuzz-dep, so we need to copy them regardless.
	deps["runtime"] = true
	deps["syscall"] = true
	deps["time"] = true
	deps["unsafe"] = true

	if *flagOut == "" {
		*flagOut = goListProps(pkg, "Name")[0] + "-fuzz"
	}

	var err error
	workdir, err = ioutil.TempDir("", "go-fuzz-build")
	if err != nil {
		failf("failed to create temp dir: %v", err)
	}
	if *flagWork {
		fmt.Printf("workdir: %v\n", workdir)
	} else {
		defer os.RemoveAll(workdir)
	}

	if deps["runtime/cgo"] {
		// Trick go command into thinking that it has up-to-date sources for cmd/cgo.
		cgoDir := filepath.Join(workdir, "src", "cmd", "cgo")
		if err := os.MkdirAll(cgoDir, 0700); err != nil {
			failf("failed to create temp dir: %v", err)
		}
		src := "// +build never\npackage main\n"
		if err := ioutil.WriteFile(filepath.Join(cgoDir, "fake.go"), []byte(src), 0600); err != nil {
			failf("failed to write temp file: %v", err)
		}
	}
	copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", "tool"), filepath.Join(workdir, "pkg", "tool"), true, nil)
	if _, err := os.Stat(filepath.Join(os.Getenv("GOROOT"), "pkg", "include")); err == nil {
		copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", "include"), filepath.Join(workdir, "pkg", "include"), true, nil)
	} else {
		// Cross-compilation is not implemented.
		copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", runtime.GOOS+"_"+runtime.GOARCH), filepath.Join(workdir, "pkg", runtime.GOOS+"_"+runtime.GOARCH), true, nil)
	}
	lits := make(map[string]bool)
	blocks := make(map[string][]Block)
	for p := range deps {
		clonePackage(workdir, p, lits, blocks)
	}
	createFuzzMain(pkg, lits)

	cmd := exec.Command("go", "build", "-tags", "gofuzz", "-o", *flagOut, mainPkg)
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GOROOT") {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "GOROOT="+workdir)
	if out, err := cmd.CombinedOutput(); err != nil {
		failf("failed to execute go build: %v\n%v", err, string(out))
	}

	coverf, err := os.Create(*flagOut + ".cover")
	if err != nil {
		failf("failed to create output file: %v", err)
	}
	if err := json.NewEncoder(coverf).Encode(blocks); err != nil {
		failf("failed to serialize coverage information: %v", err)
	}
	coverf.Close()
}

func testNormalBuild(pkg string) {
	var err error
	workdir, err = ioutil.TempDir("", "go-fuzz-build")
	if err != nil {
		failf("failed to create temp dir: %v", err)
	}
	if *flagWork {
		fmt.Printf("workdir: %v\n", workdir)
	} else {
		defer os.RemoveAll(workdir)
	}
	defer func() {
		workdir = ""
	}()
	createFuzzMain(pkg, nil)
	cmd := exec.Command("go", "build", "-tags", "gofuzz", "-o", filepath.Join(workdir, "bin"), mainPkg)
	cmd.Env = append([]string{"GOPATH=" + workdir + ":" + os.Getenv("GOPATH")}, os.Environ()...)
	if out, err := cmd.CombinedOutput(); err != nil {
		failf("failed to execute go build: %v\n%v", err, string(out))
	}
}

func createFuzzMain(pkg string, lits map[string]bool) {
	if err := os.MkdirAll(filepath.Join(workdir, "src", mainPkg), 0700); err != nil {
		failf("failed to create temp dir: %v", err)
	}
	litBuf := new(bytes.Buffer)
	for lit := range lits {
		fmt.Fprintf(litBuf, "\t%v,\n", lit)
	}
	src := fmt.Sprintf(mainSrc, pkg, *flagFunc, litBuf.String())
	if err := ioutil.WriteFile(filepath.Join(workdir, "src", mainPkg, "main.go"), []byte(src), 0600); err != nil {
		failf("failed to write temp file: %v", err)
	}
}

func clonePackage(workdir, pkg string, lits map[string]bool, blocks map[string][]Block) {
	dir := goListProps(pkg, "Dir")[0]
	if !strings.HasSuffix(dir, pkg) {
		failf("package dir '%v' does not end with import path '%v'", dir, pkg)
	}
	newDir := filepath.Join(workdir, "src", pkg)
	copyDir(dir, newDir, false, isSourceFile)
	ignore := map[string]bool{
		"runtime":       true, // lots of non-determinism and irrelevant code paths (e.g. different paths in mallocgc, chans and maps)
		"unsafe":        true, // nothing to see here (also creates import cycle with go-fuzz-dep)
		"errors":        true, // nothing to see here (also creates import cycle with go-fuzz-dep)
		"syscall":       true, // creates import cycle with go-fuzz-dep (and probably nothing to see here)
		"sync":          true, // non-deterministic and not interesting (also creates import cycle with go-fuzz-dep)
		"sync/atomic":   true, // not interesting (also creates import cycle with go-fuzz-dep)
		"time":          true, // creates import cycle with go-fuzz-dep
		"runtime/cgo":   true, // why would we instrument it?
		"runtime/pprof": true, // why would we instrument it?
		"runtime/race":  true, // why would we instrument it?
	}
	nolits := map[string]bool{
		"math":    true,
		"os":      true,
		"unicode": true,
	}
	if ignore[pkg] {
		return
	}
	if nolits[pkg] {
		lits = nil
	}
	files, err := ioutil.ReadDir(newDir)
	if err != nil {
		failf("failed to scan dir '%v': %v", dir, err)
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".go") {
			continue
		}
		fn := filepath.Join(newDir, f.Name())
		newFn := fn + ".cover"
		instrument(pkg, filepath.Join(pkg, f.Name()), fn, newFn, lits, blocks, true)
		err := os.Rename(newFn, fn)
		if err != nil {
			failf("failed to rename file: %v", err)
		}
	}
}

func copyDir(dir, newDir string, rec bool, pred func(string) bool) {
	if err := os.MkdirAll(newDir, 0700); err != nil {
		failf("failed to create temp dir: %v", err)
	}
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		failf("failed to scan dir '%v': %v", dir, err)
	}
	for _, f := range files {
		if f.IsDir() {
			if rec {
				copyDir(filepath.Join(dir, f.Name()), filepath.Join(newDir, f.Name()), rec, pred)
			}
			continue
		}
		if pred != nil && !pred(f.Name()) {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			failf("failed to read file: %v", err)
		}
		if err := ioutil.WriteFile(filepath.Join(newDir, f.Name()), data, 0700); err != nil {
			failf("failed to write temp file: %v", err)
		}
	}
}

func goListList(pkg, what string) []string {
	templ := fmt.Sprintf("{{range .%v}}{{.}}|{{end}}", what)
	out, err := exec.Command("go", "list", "-tags", "gofuzz", "-f", templ, pkg).CombinedOutput()
	if err != nil {
		failf("failed to execute 'go list -f \"%v\" %v': %v\n%v", templ, pkg, err, string(out))
	}
	if len(out) < 2 {
		failf("go list output is empty")
	}
	out = out[:len(out)-2]
	return strings.Split(string(out), "|")
}

func goListProps(pkg string, props ...string) []string {
	templ := ""
	for _, p := range props {
		templ += fmt.Sprintf("{{.%v}}|", p)
	}
	out, err := exec.Command("go", "list", "-tags", "gofuzz", "-f", templ, pkg).CombinedOutput()
	if err != nil {
		failf("failed to execute 'go list -f \"%v\" %v': %v\n%v", templ, pkg, err, string(out))
	}
	if len(out) == 0 {
		failf("go list output is empty")
	}
	out = out[:len(out)-1]
	return strings.Split(string(out), "|")
}

func failf(str string, args ...interface{}) {
	if !*flagWork && workdir != "" {
		os.RemoveAll(workdir)
	}
	fmt.Fprintf(os.Stderr, str+"\n", args...)
	os.Exit(1)
}

func isSourceFile(f string) bool {
	return (strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go")) ||
		strings.HasSuffix(f, ".s") ||
		strings.HasSuffix(f, ".S") ||
		strings.HasSuffix(f, ".c") ||
		strings.HasSuffix(f, ".h") ||
		strings.HasSuffix(f, ".cxx") ||
		strings.HasSuffix(f, ".cpp") ||
		strings.HasSuffix(f, ".c++") ||
		strings.HasSuffix(f, ".cc")
}

func isHeaderFile(f string) bool {
	return strings.HasSuffix(f, ".h")
}

var mainSrc = `
package main

import (
	target "%v"
	dep "github.com/dvyukov/go-fuzz/go-fuzz-dep"
)

func main() {
	dep.Main(target.%v, lits)
}

var lits = []string{
%v
}
`
