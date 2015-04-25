package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	flagOut  = flag.String("o", "", "output file")
	flagFunc = flag.String("func", "Fuzz", "entry function")

	workdir string
)

const (
	mainPkg = "go-fuzz-main"
)

// Copies the package with all dependent packages into a temp dir,
// instruments Go source files there and builds setting GOROOT to the temp dir.
func main() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		failf("usage: go-fuzz-build pkg")
	}

	pkg := flag.Arg(0)

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
	defer os.RemoveAll(workdir)

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
	copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", "tool"), filepath.Join(workdir, "pkg", "tool"), false, true)
	copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", "include"), filepath.Join(workdir, "pkg", "include"), false, true)
	for p := range deps {
		clonePackage(workdir, p)
	}
	createFuzzMain(pkg)

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
}

func testNormalBuild(pkg string) {
	var err error
	workdir, err = ioutil.TempDir("", "go-fuzz-build")
	if err != nil {
		failf("failed to create temp dir: %v", err)
	}
	defer func() {
		os.RemoveAll(workdir)
		workdir = ""
	}()
	createFuzzMain(pkg)
	cmd := exec.Command("go", "build", "-tags", "gofuzz", "-o", filepath.Join(workdir, "bin"), mainPkg)
	cmd.Env = append([]string{"GOPATH=" + workdir + ":" + os.Getenv("GOPATH")}, os.Environ()...)
	if out, err := cmd.CombinedOutput(); err != nil {
		failf("failed to execute go build: %v\n%v", err, string(out))
	}
}

func createFuzzMain(pkg string) {
	if err := os.MkdirAll(filepath.Join(workdir, "src", mainPkg), 0700); err != nil {
		failf("failed to create temp dir: %v", err)
	}
	src := fmt.Sprintf(mainSrc, pkg, *flagFunc)
	if err := ioutil.WriteFile(filepath.Join(workdir, "src", mainPkg, "main.go"), []byte(src), 0600); err != nil {
		failf("failed to write temp file: %v", err)
	}
}

func clonePackage(workdir, pkg string) {
	dir := goListProps(pkg, "Dir")[0]
	if !strings.HasSuffix(dir, pkg) {
		failf("package dir '%v' does not end with import path '%v'", dir, pkg)
	}
	newDir := filepath.Join(workdir, "src", pkg)
	copyDir(dir, newDir, true, false)
	ignore := []string{
		"runtime",       // lots of non-determinism and irrelevant code paths (e.g. different paths in mallocgc, chans and maps)
		"unsafe",        // nothing to see here (also creates import cycle with go-fuzz-dep)
		"errors",        // nothing to see here (also creates import cycle with go-fuzz-dep)
		"syscall",       // creates import cycle with go-fuzz-dep (and probably nothing to see here)
		"sync",          // non-deterministic and not interesting (also creates import cycle with go-fuzz-dep)
		"sync/atomic",   // not interesting (also creates import cycle with go-fuzz-dep)
		"time",          // creates import cycle with go-fuzz-dep
		"runtime/cgo",   // why would we instrument it?
		"runtime/pprof", // why would we instrument it?
		"runtime/race",  // why would we instrument it?
	}
	for _, p := range ignore {
		if pkg == p {
			return
		}
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
		instrument(fn, newFn)
		err := os.Rename(newFn, fn)
		if err != nil {
			failf("failed to rename file: %v", err)
		}
	}
}

func copyDir(dir, newDir string, src, rec bool) {
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
				copyDir(filepath.Join(dir, f.Name()), filepath.Join(newDir, f.Name()), src, rec)
			}
			continue
		}
		if src && !isSourceFile(f.Name()) {
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
	if workdir != "" {
		os.RemoveAll(workdir)
	}
	fmt.Fprintf(os.Stderr, str+"\n", args...)
	os.Exit(1)
}

func isSourceFile(f string) bool {
	return strings.HasSuffix(f, ".go") ||
		strings.HasSuffix(f, ".s") ||
		strings.HasSuffix(f, ".S") ||
		strings.HasSuffix(f, ".c") ||
		strings.HasSuffix(f, ".h") ||
		strings.HasSuffix(f, ".cxx") ||
		strings.HasSuffix(f, ".cpp") ||
		strings.HasSuffix(f, ".c++") ||
		strings.HasSuffix(f, ".cc")
}

var mainSrc = `
package main

import (
	target "%v"
	dep "github.com/dvyukov/go-fuzz/go-fuzz-dep"
)

func main() {
	dep.Main(target.%v)
}
`
