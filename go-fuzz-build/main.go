// Copyright 2015 go-fuzz project authors. All rights reserved.
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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"

	. "github.com/dvyukov/go-fuzz/internal/go-fuzz-types"
)

var (
	flagTag       = flag.String("tags", "", "a space-separated list of build tags to consider satisfied during the build")
	flagOut       = flag.String("o", "", "output file")
	flagFunc      = flag.String("func", "Fuzz", "entry function")
	flagWork      = flag.Bool("work", false, "don't remove working directory")
	flagCPU       = flag.Bool("cpuprofile", false, "generate cpu profile in cpu.pprof")
	flagLibFuzzer = flag.Bool("libfuzzer", false, "output static archive for use with libFuzzer")
)

func makeTags() string {
	tags := "gofuzz"
	if *flagLibFuzzer {
		tags += " " + "gofuzz_libfuzzer"
	}
	if len(*flagTag) > 0 {
		tags += " " + *flagTag
	}
	return tags
}

// main copies the package with all dependent packages into a temp dir,
// instruments Go source files there, and builds setting GOROOT to the temp dir.
func main() {
	flag.Parse()
	c := new(Context)

	if flag.NArg() > 1 {
		c.failf("usage: go-fuzz-build [pkg]")
	}

	pkg := "."
	if flag.NArg() == 1 {
		pkg = flag.Arg(0)
	}

	c.startProfiling()  // start pprof as requested
	c.loadPkg(pkg)      // load and typecheck pkg
	c.getEnv()          // discover GOROOT, GOPATH
	c.loadStd()         // load standard library
	c.calcIgnore()      // calculate set of packages to ignore
	c.makeWorkdir()     // create workdir
	defer c.cleanup()   // delete workdir as needed, etc.
	c.populateWorkdir() // copy tools and packages to workdir as needed

	if *flagOut == "" {
		// TODO: Context method
		ext := ".zip"
		if *flagLibFuzzer {
			ext = ".a"
		}
		*flagOut = c.fuzzpkg.Name + "-fuzz" + ext
	}

	// Gather literals, instrument, and compile.
	// Order matters here!
	// buildInstrumentedBinary (and instrumentPackages) modify the AST.
	// (We don't want to re-parse and re-typecheck every time, for performance.)
	// So we gather literals first, while the AST is pristine.
	// Then we add coverage and build.
	// Then we add sonar and build.
	// TODO: migrate to use cmd/internal/edit instead of AST modification.
	// This has several benefits: (1) It is easier to work with.
	// (2) 'go cover' has switched to it; we would get the benefit of
	// upstream bug fixes, of which there has been at least one (around gotos and labels).
	// (3) It leaves the AST intact, so we are less order-sensitive.
	// The primary blocker is that we want good line numbers for when we find crashers.
	// go/printer handles this automatically using Mode printer.SourcePos.
	// We'd need to implement that support ourselves. (It's do-able but non-trivial.)
	// See also https://golang.org/issue/29824.
	lits := c.gatherLiterals()
	var blocks, sonar []CoverBlock

	if *flagLibFuzzer {
		archive := c.buildInstrumentedBinary(&blocks, nil)
		err := os.Rename(archive, *flagOut)
		if err != nil {
			c.failf("failed to rename file: %v", err)
		}
		return
	}

	coverBin := c.buildInstrumentedBinary(&blocks, nil)
	sonarBin := c.buildInstrumentedBinary(nil, &sonar)
	metaData := c.createMeta(lits, blocks, sonar)
	defer func() {
		os.Remove(coverBin)
		os.Remove(sonarBin)
		os.Remove(metaData)
	}()

	outf, err := os.Create(*flagOut)
	if err != nil {
		c.failf("failed to create output file: %v", err)
	}
	zipw := zip.NewWriter(outf)
	zipFile := func(name, datafile string) {
		w, err := zipw.Create(name)
		if err != nil {
			c.failf("failed to create zip file: %v", err)
		}
		f, err := os.Open(datafile)
		if err != nil {
			c.failf("failed to open data file %v", datafile)
		}
		if _, err := io.Copy(w, f); err != nil {
			c.failf("failed to write %v to zip file: %v", datafile, err)
		}
		// best effort: close and remove our temp file
		f.Close()
		os.Remove(datafile)
	}
	zipFile("cover.exe", coverBin)
	zipFile("sonar.exe", sonarBin)
	zipFile("metadata", metaData)
	if err := zipw.Close(); err != nil {
		c.failf("failed to close zip file: %v", err)
	}
	if err := outf.Close(); err != nil {
		c.failf("failed to close out file: %v", err)
	}
}

// Context holds state for a go-fuzz-build run.
type Context struct {
	pkgpath string              // import path of package containing Fuzz function
	pkgs    []*packages.Package // typechecked root packages
	fuzzpkg *packages.Package   // typechecked package containing Fuzz function

	fuzzsig *types.Signature // signature of the fuzz function

	std    map[string]bool // set of packages in the standard library
	ignore map[string]bool // set of packages to ignore during instrumentation

	workdir string
	GOROOT  string
	GOPATH  string

	cpuprofile *os.File
}

// getEnv determines GOROOT and GOPATH and updates c accordingly.
func (c *Context) getEnv() {
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
		// TODO: make a single call ("go env GOROOT GOPATH") instead
		out, err := exec.Command("go", "env", k).CombinedOutput()
		if err != nil || len(out) == 0 {
			c.failf("%s is not set and failed to locate it: 'go env %s' returned '%s' (%v)", k, k, out, err)
		}
		env[k] = strings.TrimSpace(string(out))
	}
	c.GOROOT = env["GOROOT"]
	c.GOPATH = env["GOPATH"]
}

// startProfiling starts pprof profiling, if requested.
func (c *Context) startProfiling() {
	if !*flagCPU {
		return
	}
	var err error
	c.cpuprofile, err = os.Create("cpu.pprof")
	if err != nil {
		c.failf("could not create cpu profile: %v", err)
	}
	pprof.StartCPUProfile(c.cpuprofile)
}

// loadPkg loads, parses, and typechecks pkg (the package containing the Fuzz function),
// go-fuzz-dep, and their dependencies.
func (c *Context) loadPkg(pkg string) {
	// Before we do anything else, resolve the target package.
	target, err := packages.Load(nil, pkg)
	if err != nil {
		c.failf("could not load target package: %v", err)
	}
	if len(target) != 1 {
		c.failf("go-fuzz-build must be called with only one target package, found %d packages matching %v", len(target), pkg)
	}

	// Load, parse, and type-check all packages.
	// We'll use the type information later.
	// This also provides better error messages in the case
	// of invalid code than trying to compile instrumented code.
	cfg := &packages.Config{
		Mode:       packages.LoadAllSyntax,
		BuildFlags: []string{"-tags", makeTags()},
		// use custom ParseFile in order to get comments
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
	}
	// We need to load:
	// * the target package, obviously
	// * go-fuzz-dep, since we use it for instrumentation
	// * fmt, strings, os, and runtime, since the generated main function for fuzz.F users requires them
	initial, err := packages.Load(cfg, pkg, "github.com/dvyukov/go-fuzz/go-fuzz-dep", "fmt", "strings", "os", "runtime")
	if err != nil {
		c.failf("could not load packages: %v", err)
	}

	// Stop if any package had errors.
	if packages.PrintErrors(initial) > 0 {
		c.failf("typechecking of %v failed", pkg)
	}

	c.pkgs = initial

	// Find the package containing the Fuzz function.
	for _, pkg := range initial {
		if pkg.PkgPath == target[0].PkgPath {
			c.fuzzpkg = pkg
			break
		}
	}
	if c.fuzzpkg == nil {
		c.failf("could not determine fuzz package! please file an issue with reproduction instructions.")
	}

	// Check fuzz function signature.
	obj := c.fuzzpkg.Types.Scope().Lookup(*flagFunc)
	if obj == nil {
		c.failf("could not find fuzz function %s", *flagFunc)
	}
	sigType := obj.Type()
	sig, ok := sigType.(*types.Signature)
	if !ok {
		c.failf("%s is not a function", *flagFunc)
	}
	if sig.Variadic() {
		c.failf("fuzz functions cannot be variadic")
	}
	if !isLegacyFuzzSig(sig) && !isFuzzFSig(sig) {
		c.failf("fuzz function must have one of these signatures:\n\n" +
			"\tfunc Fuzz(data []byte) int\n\n" +
			"or\n\n" +
			"\timport \"github.com/dvyukov/go-fuzz\"\n\n" +
			"\tfunc Fuzz(f fuzz.F, data []byte)\n")
	}
	if isFuzzFSig(sig) && *flagLibFuzzer {
		c.failf("-libfuzzer is incompatible with using fuzz.F; use 'func Fuzz(data []byte) int' instead")
	}
	c.fuzzsig = sig
}

// isLegacyFuzzSig reports whether sig is of the form
//   func FuzzFunc(data []byte) int
func isLegacyFuzzSig(sig *types.Signature) bool {
	return tupleHasTypes(sig.Params(), "[]byte") && tupleHasTypes(sig.Results(), "int")
}

// isFuzzFSig reports whether sig is of the form
//   func FuzzFunc(f fuzz.F, data []byte)
func isFuzzFSig(sig *types.Signature) bool {
	return tupleHasTypes(sig.Params(), "github.com/dvyukov/go-fuzz.F", "[]byte") && tupleHasTypes(sig.Results())
}

// tupleHasTypes reports whether tuple is composed of
// elements with exactly the types in types.
func tupleHasTypes(tuple *types.Tuple, types ...string) bool {
	if tuple.Len() != len(types) {
		return false
	}
	for i, t := range types {
		if tuple.At(i).Type().String() != t {
			return false
		}
	}
	return true
}

// loadStd finds the set of standard library package paths.
func (c *Context) loadStd() {
	// Find out what packages are in the standard library.
	stdpkgs, err := packages.Load(nil, "std")
	if err != nil {
		c.failf("could not load standard library: %v", err)
	}
	c.std = dependencies(stdpkgs)
}

// makeWorkdir creates the workdir, logging as requested.
func (c *Context) makeWorkdir() {
	// TODO: make workdir stable, so that we can use cmd/go's build cache?
	// See https://github.com/golang/go/issues/29430.
	var err error
	c.workdir, err = ioutil.TempDir("", "go-fuzz-build")
	if err != nil {
		c.failf("failed to create temp dir: %v", err)
	}
	if *flagWork {
		fmt.Printf("workdir: %v\n", c.workdir)
	}
}

// cleanup ensures a clean exit. It should be called on all (controllable) exit paths.
func (c *Context) cleanup() {
	if !*flagWork && c.workdir != "" {
		os.RemoveAll(c.workdir)
	}
	if c.cpuprofile != nil {
		pprof.StopCPUProfile()
		c.cpuprofile.Close()
	}
}

// populateWorkdir prepares workdir for builds.
func (c *Context) populateWorkdir() {
	// TODO: instead of reconstructing the world,
	// can we use a bunch of replace directives in a go.mod?

	// TODO: make all this I/O concurrent (up to a limit).
	// It's a non-trivial part of build time.
	// Question: Do it here or in copyDir?

	// TODO: See if we can avoid making toolchain copies,
	// using some combination of env vars and toolexec.
	if *flagLibFuzzer {
		c.copyDir(filepath.Join(c.GOROOT, "src", "runtime", "cgo"), filepath.Join(c.workdir, "goroot", "src", "runtime", "cgo"))
	}
	c.copyDir(filepath.Join(c.GOROOT, "pkg", "tool"), filepath.Join(c.workdir, "goroot", "pkg", "tool"))
	if _, err := os.Stat(filepath.Join(c.GOROOT, "pkg", "include")); err == nil {
		c.copyDir(filepath.Join(c.GOROOT, "pkg", "include"), filepath.Join(c.workdir, "goroot", "pkg", "include"))
	} else {
		// Cross-compilation is not implemented.
		c.copyDir(filepath.Join(c.GOROOT, "pkg", runtime.GOOS+"_"+runtime.GOARCH), filepath.Join(c.workdir, "goroot", "pkg", runtime.GOOS+"_"+runtime.GOARCH))
	}

	// Clone our package, go-fuzz-deps, and all dependencies.
	// TODO: we might not need to do this for all packages.
	// We know that we'll be writing out instrumented Go code later;
	// we could instead just os.MkdirAll and copy non-Go files here.
	// We'd still need to do a full package clone for packages that
	// we aren't instrumenting (c.ignore).
	packages.Visit(c.pkgs, nil, func(p *packages.Package) {
		c.clonePackage(p)
	})
	c.copyFuzzDep()
}

// dependencies returns the set of all packages in root packages and their dependencies.
func dependencies(root []*packages.Package) map[string]bool {
	deps := make(map[string]bool)
	packages.Visit(root, nil, func(p *packages.Package) { deps[p.PkgPath] = true })
	return deps
}

func (c *Context) createMeta(lits map[Literal]struct{}, blocks []CoverBlock, sonar []CoverBlock) string {
	meta := MetaData{Blocks: blocks, Sonar: sonar}
	for k := range lits {
		meta.Literals = append(meta.Literals, k)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		c.failf("failed to serialize meta information: %v", err)
	}
	f := c.tempFile()
	c.writeFile(f, data)
	return f
}

func (c *Context) buildInstrumentedBinary(blocks *[]CoverBlock, sonar *[]CoverBlock) string {
	c.instrumentPackages(blocks, sonar)
	mainPkg := c.createFuzzMain()
	outf := c.tempFile()
	args := []string{"build", "-tags", makeTags()}
	if *flagLibFuzzer {
		args = append(args, "-buildmode=c-archive")
	}
	args = append(args, "-o", outf, mainPkg)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(),
		"GOROOT="+filepath.Join(c.workdir, "goroot"),
		"GOPATH="+filepath.Join(c.workdir, "gopath"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		c.failf("failed to execute go build: %v\n%v", err, string(out))
	}
	return outf
}

func (c *Context) calcIgnore() {
	// No reason to instrument these.
	c.ignore = map[string]bool{
		"runtime/cgo":   true,
		"runtime/pprof": true,
		"runtime/race":  true,
	}

	// Roots: must not instrument these, nor any of their dependencies, to avoid import cycles.
	// Fortunately, these are mostly packages that are non-deterministic,
	// noisy (because they are low level), and/or not interesting.
	// We could manually maintain this list, but that makes go-fuzz-build
	// fragile in the face of internal standard library package changes.
	roots := c.packagesNamed("runtime", "github.com/dvyukov/go-fuzz/go-fuzz-dep")
	packages.Visit(roots, func(p *packages.Package) bool {
		c.ignore[p.PkgPath] = true
		return true
	}, nil)
}

func (c *Context) gatherLiterals() map[Literal]struct{} {
	nolits := map[string]bool{
		"math":    true,
		"os":      true,
		"unicode": true,
	}

	lits := make(map[Literal]struct{})
	visit := func(pkg *packages.Package) {
		if c.ignore[pkg.PkgPath] || nolits[pkg.PkgPath] {
			return
		}
		for _, f := range pkg.Syntax {
			ast.Walk(&LiteralCollector{lits: lits, ctxt: c}, f)
		}
	}

	packages.Visit(c.pkgs, nil, visit)
	return lits
}

func (c *Context) copyFuzzDep() {
	// Standard library packages can't depend on non-standard ones.
	// So we pretend that go-fuzz-dep is a standard one.
	// go-fuzz-dep depends on go-fuzz-defs, which creates a problem.
	// Fortunately (and intentionally), go-fuzz-defs contains only constants,
	// which can be duplicated safely.
	// So we eliminate the import statement and copy go-fuzz-defs/defs.go
	// directly into the go-fuzz-dep package.
	newDir := filepath.Join(c.workdir, "goroot", "src", "go-fuzz-dep")
	c.mkdirAll(newDir)
	dep := c.packageNamed("github.com/dvyukov/go-fuzz/go-fuzz-dep")
	for _, f := range dep.GoFiles {
		data := c.readFile(f)
		// Eliminate the dot import.
		data = bytes.Replace(data, []byte(`. "github.com/dvyukov/go-fuzz/go-fuzz-defs"`), nil, -1)
		c.writeFile(filepath.Join(newDir, filepath.Base(f)), data)
	}

	defs := c.packageNamed("github.com/dvyukov/go-fuzz/go-fuzz-defs")
	for _, f := range defs.GoFiles {
		data := c.readFile(f)
		// Adjust package name to match go-fuzz-deps.
		data = bytes.Replace(data, []byte("\npackage base"), []byte("\npackage gofuzzdep"), -1)
		c.writeFile(filepath.Join(newDir, "defs.go"), data)
	}
}

func (c *Context) funcMain() []byte {
	t := mainSrc
	if *flagLibFuzzer {
		t = mainSrcLibFuzzer
	}
	dot := struct {
		Pkg     string
		Func    string
		IsFuzzF bool
	}{
		Pkg:     c.fuzzpkg.PkgPath,
		Func:    *flagFunc,
		IsFuzzF: isFuzzFSig(c.fuzzsig),
	}
	buf := new(bytes.Buffer)
	if err := t.Execute(buf, dot); err != nil {
		c.failf("could not execute template: %v", err)
	}
	return buf.Bytes()
}

func (c *Context) createFuzzMain() string {
	mainPkg := filepath.Join(c.fuzzpkg.PkgPath, "go.fuzz.main")
	path := filepath.Join(c.workdir, "gopath", "src", mainPkg)
	c.mkdirAll(path)
	c.writeFile(filepath.Join(path, "main.go"), c.funcMain())
	return mainPkg
}

func (c *Context) clonePackage(p *packages.Package) {
	root := "goroot"
	if !c.std[p.PkgPath] {
		root = "gopath"
	}
	newDir := filepath.Join(c.workdir, root, "src", p.PkgPath)
	c.mkdirAll(newDir)

	if p.PkgPath == "unsafe" {
		// Write a dummy file. go/packages explicitly returns an empty GoFiles for it,
		// for reasons that are unclear, but cmd/go wants there to be a Go file in the package.
		c.writeFile(filepath.Join(newDir, "unsafe.go"), []byte(`package unsafe`))
		return
	}

	// Copy all the source code.

	// Use GoFiles instead of CompiledGoFiles here.
	// If we use CompiledGoFiles, we end up with code that cmd/go won't compile.
	// See https://golang.org/issue/30479 and Context.instrumentPackages.
	for _, f := range p.GoFiles {
		dst := filepath.Join(newDir, filepath.Base(f))
		c.copyFile(f, dst)
	}
	for _, f := range p.OtherFiles {
		dst := filepath.Join(newDir, filepath.Base(f))
		c.copyFile(f, dst)
	}

	// TODO: do we need to look for and copy go.mod?
}

// packageNamed extracts the package listed in path.
func (c *Context) packageNamed(path string) (pkgs *packages.Package) {
	all := c.packagesNamed(path)
	if len(all) != 1 {
		c.failf("got multiple packages, requested only %v", path)
	}
	return all[0]
}

// packagesNamed extracts the packages listed in paths.
func (c *Context) packagesNamed(paths ...string) (pkgs []*packages.Package) {
	pre := func(p *packages.Package) bool {
		for _, path := range paths {
			if p.PkgPath == path {
				pkgs = append(pkgs, p)
				break
			}
		}
		return len(pkgs) < len(paths) // continue only if we have not succeeded yet
	}
	packages.Visit(c.pkgs, pre, nil)
	return pkgs
}

func (c *Context) instrumentPackages(blocks *[]CoverBlock, sonar *[]CoverBlock) {
	visit := func(pkg *packages.Package) {
		if c.ignore[pkg.PkgPath] {
			return
		}

		root := "goroot"
		if !c.std[pkg.PkgPath] {
			root = "gopath"
		}
		path := filepath.Join(c.workdir, root, "src", pkg.PkgPath) // TODO: need filepath.FromSlash for pkg.PkgPath?

		for i, fullName := range pkg.CompiledGoFiles {
			fname := filepath.Base(fullName)
			if !strings.HasSuffix(fname, ".go") {
				// This is a cgo-generated file.
				// Instrumenting it currently does not work.
				// We copied the original Go file as part of copyPackageRewrite,
				// so we can just skip this one.
				// See https://golang.org/issue/30479.
				continue
			}
			f := pkg.Syntax[i]

			// TODO: rename trimComments?
			f.Comments = trimComments(f, pkg.Fset)

			buf := new(bytes.Buffer)
			content := c.readFile(fullName)
			buf.Write(initialComments(content)) // Retain '// +build' directives.
			instrument(pkg.PkgPath, fullName, pkg.Fset, f, pkg.TypesInfo, buf, blocks, sonar)
			tmp := c.tempFile()
			c.writeFile(tmp, buf.Bytes())
			outpath := filepath.Join(path, fname)
			if runtime.GOOS == "windows" {
				os.Remove(outpath)
			}
			err := os.Rename(tmp, outpath)
			if err != nil {
				c.failf("failed to rename file: %v", err)
			}
		}
	}

	packages.Visit(c.pkgs, nil, visit)
}

func (c *Context) copyDir(dir, newDir string) {
	c.mkdirAll(newDir)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		c.failf("failed to scan dir '%v': %v", dir, err)
	}
	for _, f := range files {
		if f.IsDir() {
			c.copyDir(filepath.Join(dir, f.Name()), filepath.Join(newDir, f.Name()))
			continue
		}
		src := filepath.Join(dir, f.Name())
		dst := filepath.Join(newDir, f.Name())
		c.copyFile(src, dst)
	}
}

func (c *Context) copyFile(src, dst string) {
	r, err := os.Open(src)
	if err != nil {
		c.failf("copyFile: could not read %v", src, err)
	}
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		c.failf("copyFile: could not write %v: %v", dst, err)
	}
	if _, err := io.Copy(w, r); err != nil {
		c.failf("copyFile: copying failed: %v", err)
	}
	if err := r.Close(); err != nil {
		c.failf("copyFile: closing %v failed: %v", src, err)
	}
	if err := w.Close(); err != nil {
		c.failf("copyFile: closing %v failed: %v", dst, err)
	}
}

func (c *Context) failf(str string, args ...interface{}) {
	c.cleanup()
	fmt.Fprintf(os.Stderr, str+"\n", args...)
	os.Exit(1)
}

func (c *Context) tempFile() string {
	outf, err := ioutil.TempFile("", "go-fuzz")
	if err != nil {
		c.failf("failed to create temp file: %v", err)
	}
	outf.Close()
	return outf.Name()
}

func (c *Context) readFile(name string) []byte {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		c.failf("failed to read temp file: %v", err)
	}
	return data
}

func (c *Context) writeFile(name string, data []byte) {
	if err := ioutil.WriteFile(name, data, 0700); err != nil {
		c.failf("failed to write temp file: %v", err)
	}
}

func (c *Context) mkdirAll(dir string) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		c.failf("failed to create temp dir: %v", err)
	}
}

var mainSrc = template.Must(template.New("main").Parse(`
package main

import (
	target "{{.Pkg}}"
	dep "github.com/dvyukov/go-fuzz/go-fuzz-dep"
	{{ if .IsFuzzF }}
	"fmt"
	"os"
	"runtime"
	"strings"
	{{ end }}
)


func main() {
	{{ if .IsFuzzF }}
	dep.Main(Fuzz)
	{{ else }}
	dep.Main(target.{{.Func}})
	{{ end }}
}

{{ if .IsFuzzF }}
func Fuzz(data []byte) int {
	f := new(F)
	f.name = "{{.Func}}"
	done := make(chan bool)
	go func() {
		defer close(done)
		target.{{.Func}}(f, data)
	}()
	<-done
	return f.rc
}

// See github.com/dvukov/go-fuzz.F for documentation of F and its methods.
type F struct {
	name string
	rc int
}

func (f *F) FailNow() {
	// TODO: communicate this to go-fuzz instead of dying,
	// so that we don't have to restart on failure.
	os.Exit(1)
}

func (f *F) SkipNow() {
	f.rc = -1
	runtime.Goexit()
}

func (f *F) Name() string {
	return f.name
}

func (f *F) Log(args ...interface{}) {
	fmt.Println(args...)
}

func (f *F) Logf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	if !strings.HasSuffix(format, "\n") {
		fmt.Println()
	}
}

func (f *F) Interesting() {
	f.rc++
}

// ------------
// Functions below are implemented in terms of functions above.
// ------------

func (f *F) Error(args ...interface{}) {
	f.Fatal(args...)
}

func (f *F) Errorf(format string, args ...interface{}) {
	f.Fatalf(format, args...)
}

func (f *F) Fail() {
	f.FailNow()
}

func (f *F) Fatal(args ...interface{}) {
	f.Log(args...)
	f.FailNow()
}

func (f *F) Fatalf(format string, args ...interface{}) {
	f.Logf(format, args...)
	f.FailNow()
}

func (f *F) Failed() bool {
	return false
}

func (f *F) Skip(args ...interface{}) {
	f.SkipNow()
}

func (f *F) Skipf(format string, args ...interface{}) {
	f.SkipNow()
}

func (f *F) Skipped() bool {
	return false
}

func (f *F) Helper() {}
{{ end }}
`))

var mainSrcLibFuzzer = template.Must(template.New("main").Parse(`
package main

import (
	"unsafe"
	"reflect"
	target "{{.Pkg}}"
	dep "github.com/dvyukov/go-fuzz/go-fuzz-dep"
)

// #cgo CFLAGS: -Wall -Werror
// #ifdef __linux__
// __attribute__((weak, section("__libfuzzer_extra_counters")))
// #else
// #error Currently only Linux is supported
// #endif
// unsigned char GoFuzzCoverageCounters[65536];
import "C"

//export LLVMFuzzerInitialize
func LLVMFuzzerInitialize(argc uintptr, argv uintptr) int {
	dep.Initialize(unsafe.Pointer(&C.GoFuzzCoverageCounters[0]), 65536)
	return 0
}

//export LLVMFuzzerTestOneInput
func LLVMFuzzerTestOneInput(data uintptr, size uint64) int {
	sh := &reflect.SliceHeader{
	    Data: data,
	    Len:  int(size),
	    Cap:  int(size),
	}

	input := *(*[]byte)(unsafe.Pointer(sh))
	target.{{.Func}}(input)

	return 0
}

func main() {
}
`))
