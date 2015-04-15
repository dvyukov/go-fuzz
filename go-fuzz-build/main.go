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

func main() {
	flag.Parse()
	if len(flag.Args()) != 1 {
		failf("usage: go-fuzz-build pkg")
	}

	pkg := flag.Arg(0)
	deps := goListList(pkg, "Deps")
	deps = append(deps, pkg)

	if *flagOut == "" {
		*flagOut = goListProps(pkg, "Name")[0] + "-fuzz"
	}

	var err error
	workdir, err = ioutil.TempDir("", "go-fuzz-build")
	if err != nil {
		failf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(workdir)

	copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", "tool"), filepath.Join(workdir, "pkg", "tool"), true)
	copyDir(filepath.Join(os.Getenv("GOROOT"), "pkg", "include"), filepath.Join(workdir, "pkg", "include"), true)
	for _, p := range deps {
		clonePackage(workdir, p)
	}
	err = os.MkdirAll(filepath.Join(workdir, "src", "go-fuzz-main"), 0700)
	if err != nil {
		failf("failed to create temp dir: %v", err)
	}
	src := fmt.Sprintf(mainSrc, pkg, *flagFunc)
	err = ioutil.WriteFile(filepath.Join(workdir, "src", "go-fuzz-main", "main.go"), []byte(src), 0600)

	cmd := exec.Command("go", "build", "-tags", "gofuzz", "-o", *flagOut, "go-fuzz-main")
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

func clonePackage(workdir, pkg string) {
	dir := goListProps(pkg, "Dir")[0]
	if !strings.HasSuffix(dir, pkg) {
		failf("package dir '%v' does not end with import path '%v'", dir, pkg)
	}
	newDir := filepath.Join(workdir, "src", pkg)
	copyDir(dir, newDir, false)
	for _, p := range []string{"runtime", "unsafe", "syscall", "sync", "sync/atomic", "time", "runtime/cgo", "runtime/pprof", "runtime/race"} {
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

func copyDir(dir, newDir string, rec bool) {
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
				copyDir(filepath.Join(dir, f.Name()), filepath.Join(newDir, f.Name()), rec)
			}
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			failf("failed to read file: %v", err)
		}
		err = ioutil.WriteFile(filepath.Join(newDir, f.Name()), data, 0700)
		if err != nil {
			failf("failed to write file: %v", err)
		}
	}
}

func goListList(pkg, what string) []string {
	out, err := exec.Command("go", "list", "-f", fmt.Sprintf("{{range .%v}}{{.}}|{{end}}", what), pkg).CombinedOutput()
	if err != nil {
		failf("failed to execute go list: %v", err)
	}
	if len(out) == 0 {
		failf("go list output is empty")
	}
	out = out[:len(out)-1]
	return strings.Split(string(out), "|")
}

func goListProps(pkg string, props ...string) []string {
	templ := ""
	for _, p := range props {
		templ += fmt.Sprintf("{{.%v}}|", p)
	}
	out, err := exec.Command("go", "list", "-f", templ, pkg).CombinedOutput()
	if err != nil {
		failf("failed to execute go list: %v", err)
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
