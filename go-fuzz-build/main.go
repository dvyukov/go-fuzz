// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

var (
	flagTag  = flag.String("tags", "", "a space-separated list of build tags to consider satisfied during the build")
	flagOut  = flag.String("o", "", "output file")
	flagFunc = flag.String("func", "Fuzz", "entry function")
	flagWork = flag.Bool("work", false, "don't remove working directory")

	workdir string
	GOROOT  string
	GOPATH  string
)

func makeTags() string {
	tags := "gofuzz"
	if len(*flagTag) > 0 {
		tags += " " + *flagTag
	}
	return tags
}

// Copies the package with all dependent packages into a temp dir,
// instruments Go source files there and builds setting GOROOT to the temp dir.
func main() {
	flag.Parse()
	if len(flag.Args()) != 1 || len(flag.Arg(0)) == 0 {
		failf("usage: go-fuzz-build pkg")
	}
	env := map[string]string{
		"GOROOT": "",
		"GOPATH": "",
	}
	for k := range env {
		v := os.Getenv(k)
		if v != "" {
			env[k] = v
			continue
		}
		out, err := exec.Command("go", "env", k).CombinedOutput()
		if err != nil || len(out) == 0 {
			failf("%s is not set and failed to locate it: 'go env %s' returned '%s' (%v)", k, k, out, err)
		}
		env[k] = strings.TrimSpace(string(out))
	}
	GOROOT = env["GOROOT"]
	GOPATH = env["GOPATH"]

	pkg := flag.Arg(0)
	if pkg[0] == '.' {
		failf("relative import paths are not supported, please specify full package name")
	}

	// To produce error messages (this is much faster and gives correct line numbers).
	if !strings.Contains(pkg, "internal") {
		// Can't test normal build for internal packages.
		// For internal packages we need to create main package in the same dir
		// as the internal package. But since we don't mess with real GOPATH
		// we can't do that.
		testNormalBuild(pkg)
	}

	deps := make(map[string]bool)
	for _, p := range goListList(pkg, "Deps") {
		deps[p] = goListBool(p, "Standard")
	}
	deps[pkg] = goListBool(pkg, "Standard")
	// Also add all go-fuzz-dep dependencies.
	for _, p := range goListList("github.com/dvyukov/go-fuzz/go-fuzz-dep", "Deps") {
		if p == "go-fuzz-defs" {
			continue
		}
		deps[p] = true
	}

	lits := make(map[Literal]struct{})
	var blocks, sonar []CoverBlock
	sonarBin := buildInstrumentedBinary(pkg, deps, nil, nil, &sonar)
	coverBin := buildInstrumentedBinary(pkg, deps, lits, &blocks, nil)
	metaData := createMeta(lits, blocks, sonar)
	defer func() {
		os.Remove(coverBin)
		os.Remove(sonarBin)
		os.Remove(metaData)
	}()

	if *flagOut == "" {
		*flagOut = goListProps(pkg, "Name")[0] + "-fuzz.zip"
	}
	outf, err := os.Create(*flagOut)
	if err != nil {
		failf("failed to create output file: %v", err)
	}
	zipw := zip.NewWriter(outf)
	zipFile := func(name, datafile string) {
		w, err := zipw.Create(name)
		if err != nil {
			failf("failed to create zip file: %v", err)
		}
		if _, err := w.Write(readFile(datafile)); err != nil {
			failf("failed to write to zip file: %v", err)
		}
	}
	zipFile("cover.exe", coverBin)
	zipFile("sonar.exe", sonarBin)
	zipFile("metadata", metaData)
	if err := zipw.Close(); err != nil {
		failf("failed to close zip file: %v", err)
	}
	if err := outf.Close(); err != nil {
		failf("failed to close out file: %v", err)
	}
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
	copyFuzzDep(workdir, false)
	mainPkg := createFuzzMain(pkg)
	cmd := exec.Command("go", "build", "-tags", makeTags(), "-o", filepath.Join(workdir, "bin"), mainPkg)
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GOPATH") {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "GOPATH="+GOPATH+string(os.PathListSeparator)+filepath.Join(workdir, "gopath"))
	if out, err := cmd.CombinedOutput(); err != nil {
		failf("failed to execute go build: %v\n%v", err, string(out))
	}
}

func createMeta(lits map[Literal]struct{}, blocks []CoverBlock, sonar []CoverBlock) string {
	meta := MetaData{Blocks: blocks, Sonar: sonar}
	for k := range lits {
		meta.Literals = append(meta.Literals, k)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		failf("failed to serialize meta information: %v", err)
	}
	f := tempFile()
	writeFile(f, data)
	return f
}

func buildInstrumentedBinary(pkg string, deps map[string]bool, lits map[Literal]struct{}, blocks *[]CoverBlock, sonar *[]CoverBlock) string {
	var err error
	workdir, err = ioutil.TempDir("", "go-fuzz-build")
	if err != nil {
		failf("failed to create temp dir: %v", err)
	}
	if *flagWork {
		fmt.Printf("workdir: %v\n", workdir)
	} else {
		defer func() {
			os.RemoveAll(workdir)
			workdir = ""
		}()
	}

	if deps["runtime/cgo"] {
		// Trick go command into thinking that it has up-to-date sources for cmd/cgo.
		cgoDir := filepath.Join(workdir, "goroot", "src", "cmd", "cgo")
		mkdirAll(cgoDir)
		src := "// +build never\npackage main\n"
		writeFile(filepath.Join(cgoDir, "fake.go"), []byte(src))
	}
	copyDir(filepath.Join(GOROOT, "pkg", "tool"), filepath.Join(workdir, "goroot", "pkg", "tool"), true, nil)
	if _, err := os.Stat(filepath.Join(GOROOT, "pkg", "include")); err == nil {
		copyDir(filepath.Join(GOROOT, "pkg", "include"), filepath.Join(workdir, "goroot", "pkg", "include"), true, nil)
	} else {
		// Cross-compilation is not implemented.
		copyDir(filepath.Join(GOROOT, "pkg", runtime.GOOS+"_"+runtime.GOARCH), filepath.Join(workdir, "goroot", "pkg", runtime.GOOS+"_"+runtime.GOARCH), true, nil)
	}
	for p, std := range deps {
		clonePackage(workdir, p, p, std)
	}
	instrumentPackages(workdir, deps, lits, blocks, sonar)
	copyFuzzDep(workdir, true)
	mainPkg := createFuzzMain(pkg)

	outf := tempFile()
	os.Remove(outf)
	outf += ".exe"
	cmd := exec.Command("go", "build", "-tags", makeTags(), "-o", outf, mainPkg)
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GOROOT") || strings.HasPrefix(v, "GOPATH") {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "GOROOT="+filepath.Join(workdir, "goroot"))
	cmd.Env = append(cmd.Env, "GOPATH="+filepath.Join(workdir, "gopath"))
	if out, err := cmd.CombinedOutput(); err != nil {
		failf("failed to execute go build: %v\n%v", err, string(out))
	}
	return outf
}

func copyFuzzDep(workdir string, std bool) {
	// In Go1.6 standard packages can't depend on non-standard ones.
	// So we pretend that go-fuzz-dep is a standard one.
	clonePackage(workdir, "github.com/dvyukov/go-fuzz/go-fuzz-dep", "go-fuzz-dep", std)
	clonePackage(workdir, "github.com/dvyukov/go-fuzz/go-fuzz-defs", "go-fuzz-defs", std)
}

func createFuzzMain(pkg string) string {
	mainPkg := filepath.Join(pkg, "go.fuzz.main")
	path := filepath.Join(workdir, "gopath", "src", mainPkg)
	mkdirAll(path)
	src := fmt.Sprintf(mainSrc, pkg, *flagFunc)
	writeFile(filepath.Join(path, "main.go"), []byte(src))
	return mainPkg
}

func clonePackage(workdir, pkg, targetPkg string, std bool) {
	dir := goListProps(pkg, "Dir")[0]
	if !strings.HasSuffix(filepath.ToSlash(dir), pkg) {
		failf("package dir '%v' does not end with import path '%v'", dir, pkg)
	}
	root := "goroot"
	if !std {
		root = "gopath"
	}
	newDir := filepath.Join(workdir, root, "src", targetPkg)
	copyDir(dir, newDir, false, isSourceFile)
}

type Package struct {
	name    string
	fset    *token.FileSet
	ast     map[string]*ast.File
	typed   *types.Package
	info    types.Info
	nimport int
	deps    []*Package
}

type Importer struct {
	pkgs map[string]*types.Package
	deps map[string]bool
}

func (imp *Importer) Import(path string) (*types.Package, error) {
	panic("must not be called")
}

func (imp *Importer) ImportFrom(path, srcDir string, mode types.ImportMode) (*types.Package, error) {
	if pkg := imp.pkgs[path]; pkg != nil {
		return pkg, nil
	}

	// Vendor hackery.
	if prefix := filepath.Join(workdir, "goroot", "src") + string(os.PathSeparator); strings.HasPrefix(srcDir, prefix) {
		srcDir = srcDir[len(prefix):]
	}
	if prefix := filepath.Join(workdir, "gopath", "src") + string(os.PathSeparator); strings.HasPrefix(srcDir, prefix) {
		srcDir = srcDir[len(prefix):]
	}
	parts := strings.Split(srcDir, string(os.PathSeparator))
	for i := 0; i <= len(parts); i++ {
		vendorPath := strings.Join(parts[:len(parts)-i], "/")
		if len(vendorPath) > 0 {
			vendorPath += "/"
		}
		vendorPath += strings.Join([]string{"vendor", path}, "/")
		if pkg := imp.pkgs[vendorPath]; pkg != nil {
			return pkg, nil
		}
	}
	failf("can't find imported package %v", path)
	return nil, nil
}

func instrumentPackages(workdir string, deps map[string]bool, lits map[Literal]struct{}, blocks *[]CoverBlock, sonar *[]CoverBlock) {
	ignore := map[string]bool{
		"runtime":                         true, // lots of non-determinism and irrelevant code paths (e.g. different paths in mallocgc, chans and maps)
		"runtime/internal/atomic":         true, // runtime depends on it
		"runtime/internal/sys":            true, // runtime depends on it
		"unsafe":                          true, // nothing to see here (also creates import cycle with go-fuzz-dep)
		"errors":                          true, // nothing to see here (also creates import cycle with go-fuzz-dep)
		"syscall":                         true, // creates import cycle with go-fuzz-dep (and probably nothing to see here)
		"internal/syscall/windows/sysdll": true, //syscall depends on it
		"sync":             true, // non-deterministic and not interesting (also creates import cycle with go-fuzz-dep)
		"sync/atomic":      true, // not interesting (also creates import cycle with go-fuzz-dep)
		"time":             true, // creates import cycle with go-fuzz-dep
		"internal/bytealg": true, // runtime depends on it
		"internal/cpu":     true, // runtime depends on it
		"internal/race":    true, // runtime depends on it
		"runtime/cgo":      true, // why would we instrument it?
		"runtime/pprof":    true, // why would we instrument it?
		"runtime/race":     true, // why would we instrument it?
	}
	if runtime.GOOS == "windows" {
		// Cross-compilation is not implemented.
		ignore["unicode/utf16"] = true                     // syscall depends on unicode/utf16
		ignore["internal/syscall/windows/registry"] = true // time depends on this
		ignore["io"] = true                                // internal/syscall/windows/registry depends on this
	}
	nolits := map[string]bool{
		"math":    true,
		"os":      true,
		"unicode": true,
	}

	var ready []*Package
	pkgs := make(map[string]*Package)
	for pkg := range deps {
		p := pkgs[pkg]
		if p == nil {
			p = &Package{name: pkg}
			pkgs[pkg] = p
		}
		for _, imp := range goListList(pkg, "Imports") {
			p1 := pkgs[imp]
			if p1 == nil {
				p1 = &Package{name: imp}
				pkgs[imp] = p1
			}
			p.nimport++
			p1.deps = append(p1.deps, p)
		}
		if p.nimport == 0 {
			ready = append(ready, p)
		}
	}
	typedPackages := make(map[string]*types.Package)
	importer := &Importer{typedPackages, deps}
	for len(ready) != 0 {
		p := ready[len(ready)-1]
		ready = ready[:len(ready)-1]

		if p.name == "unsafe" {
			typedPackages["unsafe"] = types.Unsafe
		} else {
			p.fset = token.NewFileSet()
			p.ast = make(map[string]*ast.File)
			p.info.Types = make(map[ast.Expr]types.TypeAndValue)
			root := "goroot"
			if !deps[p.name] {
				root = "gopath"
			}
			path := filepath.Join(workdir, root, "src", p.name)
			var files []*ast.File
			for _, fn := range append(goListList(p.name, "GoFiles"), goListList(p.name, "CgoFiles")...) {
				astFile, err := parser.ParseFile(p.fset, filepath.Join(path, fn), nil, parser.ParseComments)
				if err != nil {
					failf("failed to parse package %v: %v", p.name, err)
				}
				astFile.Comments = trimComments(astFile, p.fset)
				p.ast[fn] = astFile
				files = append(files, astFile)
			}

			cfg := &types.Config{
				Importer: importer,
			}
			typed, err := cfg.Check(p.name, p.fset, files, &p.info)
			if err != nil {
				failf("failed to type check package %v: %v", p.name, err)
			}
			typedPackages[p.name] = typed

			if !ignore[p.name] {
				lits1 := lits
				if nolits[p.name] {
					lits1 = nil
				}
				for fname, f := range p.ast {
					fullName := filepath.Join(path, fname)
					buf := new(bytes.Buffer)
					content := readFile(fullName)
					buf.Write(initialComments(content)) // Retain '// +build' directives.
					instrument(p.name, fname, filepath.Join(p.name, fname), p.fset, f, &p.info, buf, lits1, blocks, sonar)
					tmp := tempFile()
					if runtime.GOOS == "windows" {
						os.Remove(fullName)
					}
					writeFile(tmp, buf.Bytes())
					err := os.Rename(tmp, fullName)
					if err != nil {
						failf("failed to rename file: %v", err)
					}
				}
			}
		}

		for _, p1 := range p.deps {
			p1.nimport--
			if p1.nimport == 0 {
				ready = append(ready, p1)
			}
		}
	}
}

func copyDir(dir, newDir string, rec bool, pred func(string) bool) {
	mkdirAll(newDir)
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
		data := readFile(filepath.Join(dir, f.Name()))
		writeFile(filepath.Join(newDir, f.Name()), data)
	}
}

func goListList(pkg, what string) []string {
	templ := fmt.Sprintf("{{range .%v}}{{.}}|{{end}}", what)
	cmd := exec.Command("go", "list", "-tags", makeTags(), "-f", templ, pkg)
	cmd.Env = append(os.Environ(), "GOPATH="+GOPATH)
	out, err := cmd.CombinedOutput()
	if err != nil {
		failf("failed to execute 'go list -f %q %v': %v\n%v", templ, pkg, err, string(out))
	}
	if len(out) < 2 {
		return nil
	}
	out = out[:len(out)-2]
	return strings.Split(string(out), "|")
}

func goListProps(pkg string, props ...string) []string {
	templ := ""
	for _, p := range props {
		templ += fmt.Sprintf("{{.%v}}|", p)
	}
	cmd := exec.Command("go", "list", "-tags", makeTags(), "-f", templ, pkg)
	cmd.Env = append(os.Environ(), "GOPATH="+GOPATH)
	out, err := cmd.CombinedOutput()
	if err != nil {
		failf("failed to execute 'go list -f %q %v': %v\n%v", templ, pkg, err, string(out))
	}
	if len(out) == 0 {
		failf("goListProps: go list output is empty")
	}
	out = out[:len(out)-1]
	return strings.Split(string(out), "|")
}

func goListBool(pkg, what string) bool {
	templ := fmt.Sprintf("{{.%v}}", what)
	cmd := exec.Command("go", "list", "-tags", makeTags(), "-f", templ, pkg)
	cmd.Env = append(os.Environ(), "GOPATH="+GOPATH)
	out, err := cmd.CombinedOutput()
	if err != nil {
		failf("failed to execute 'go list -f \"%v\" %v': %v\n%v", templ, pkg, err, string(out))
	}
	return string(out) == "true\n"
}

func failf(str string, args ...interface{}) {
	if !*flagWork && workdir != "" {
		os.RemoveAll(workdir)
	}
	fmt.Fprintf(os.Stderr, str+"\n", args...)
	os.Exit(1)
}

func tempFile() string {
	outf, err := ioutil.TempFile("", "go-fuzz")
	if err != nil {
		failf("failed to create temp file: %v", err)
	}
	outf.Close()
	return outf.Name()
}

func readFile(name string) []byte {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		failf("failed to read temp file: %v", err)
	}
	return data
}

func writeFile(name string, data []byte) {
	if err := ioutil.WriteFile(name, data, 0700); err != nil {
		failf("failed to write temp file: %v", err)
	}
}

func mkdirAll(dir string) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		failf("failed to create temp dir: %v", err)
	}
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
	dep "go-fuzz-dep"
)

func main() {
	dep.Main(target.%v)
}
`
