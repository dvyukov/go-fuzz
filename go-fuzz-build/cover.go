package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

const fuzzdepPkg = "_go_fuzz_dep_"

func instrument(pkg, shortName, in, out string, lits map[Literal]struct{}, blocks *[]CoverBlock, sonar *[]CoverBlock) {
	fset := token.NewFileSet()
	content, err := ioutil.ReadFile(in)
	if err != nil {
		failf("cover: %s: %s", in, err)
	}
	parsedFile, err := parser.ParseFile(fset, in, content, parser.ParseComments)
	if err != nil {
		failf("cover: %s: %s", in, err)
	}
	parsedFile.Comments = trimComments(parsedFile, fset)

	file := &File{
		fset:      fset,
		name:      in,
		shortName: shortName,
		astFile:   parsedFile,
		blocks:    blocks,
	}
	file.addImport("github.com/dvyukov/go-fuzz/go-fuzz-dep", fuzzdepPkg, "Main")

	if lits != nil {
		ast.Walk(&LiteralCollector{lits}, file.astFile)
	}

	ast.Walk(file, file.astFile)

	if sonar != nil {
		s := &Sonar{fset: fset, name: shortName, pkg: pkg, blocks: sonar}
		ast.Walk(s, file.astFile)
	}

	fd, err := os.Create(out)
	if err != nil {
		failf("failed to create temp file: %v")
	}
	defer fd.Close()
	fd.Write(initialComments(content)) // Retain '// +build' directives.
	file.print(fd)
}

type Sonar struct {
	fset   *token.FileSet
	name   string
	pkg    string
	blocks *[]CoverBlock
}

var sonarSeq = 0

func (s *Sonar) Visit(n ast.Node) ast.Visitor {
	// TODO: detect "x&mask==0", emit sonar(x, x&^mask)
	switch nn := n.(type) {
	case *ast.BinaryExpr:
		break
	case *ast.GenDecl:
		if nn.Tok != token.VAR {
			return nil // constants and types are not interesting
		}
		return s

	case *ast.FuncDecl:
		if s.pkg == "math" && (nn.Name.Name == "Y0" || nn.Name.Name == "Y1" || nn.Name.Name == "Yn" ||
			nn.Name.Name == "J0" || nn.Name.Name == "J1" || nn.Name.Name == "Jn" ||
			nn.Name.Name == "Pow") {
			// Can't handle code there:
			// math/j0.go:93: constant 680564733841876926926749214863536422912 overflows int
			return nil
		}
		return s // recurse

	case *ast.SwitchStmt:
		if nn.Tag == nil || nn.Body == nil {
			return s // recurse
		}
		// Replace:
		//	switch a := foo(); bar(a) {
		//	case x: ...
		//	case y: ...
		//	}
		// with:
		//	switch {
		//	default:
		//		a := foo()
		//		__tmp := bar(a)
		//		switch {
		//		case __tmp == x: ...
		//		case __tmp == y: ...
		//		}
		//	}
		// The == comparisons will be instrumented later when we recurse.
		sw := new(ast.SwitchStmt)
		*sw = *nn
		var stmts []ast.Stmt
		if sw.Init != nil {
			stmts = append(stmts, sw.Init)
			sw.Init = nil
		}
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: "__go_fuzz_tmp"}}, Tok: token.DEFINE, Rhs: []ast.Expr{sw.Tag}})
		sw.Tag = nil
		stmts = append(stmts, sw)
		for _, cas1 := range sw.Body.List {
			cas := cas1.(*ast.CaseClause)
			for i, expr := range cas.List {
				cas.List[i] = &ast.BinaryExpr{X: &ast.Ident{Name: "__go_fuzz_tmp", NamePos: expr.Pos()}, Op: token.EQL, Y: expr}
			}
		}
		nn.Tag = nil
		nn.Init = nil
		nn.Body = &ast.BlockStmt{List: []ast.Stmt{&ast.CaseClause{Body: stmts}}}
		return s // recurse

	case *ast.ForStmt:
		// For condition is usually uninteresting, but produces lots of samples.
		// So we skip it if it looks boring.
		if nn.Init != nil {
			ast.Walk(s, nn.Init)
		}
		if nn.Post != nil {
			ast.Walk(s, nn.Post)
		}
		ast.Walk(s, nn.Body)
		if nn.Cond != nil {
			// Look for the following pattern:
			//	for foo := ...; foo ? ...; ... { ... }
			boring := false
			if nn.Init != nil {
				if init, ok1 := nn.Init.(*ast.AssignStmt); ok1 && init.Tok == token.DEFINE && len(init.Lhs) == 1 {
					if id, ok2 := init.Lhs[0].(*ast.Ident); ok2 {
						if bex, ok3 := nn.Cond.(*ast.BinaryExpr); ok3 {
							if x, ok4 := bex.X.(*ast.Ident); ok4 && x.Name == id.Name {
								boring = true
							}
							if x, ok4 := bex.Y.(*ast.Ident); ok4 && x.Name == id.Name {
								boring = true
							}
						}
					}
				}
			}
			if !boring {
				ast.Walk(s, nn.Cond)
			}
		}
		return nil

	default:
		return s // recurse
	}
	nn := n.(*ast.BinaryExpr)
	var flags uint8
	switch nn.Op {
	case token.EQL:
		flags = SonarEQL
		break
	case token.NEQ:
		flags = SonarNEQ
		break
	case token.LSS:
		flags = SonarLSS
		break
	case token.GTR:
		flags = SonarGTR
		break
	case token.LEQ:
		flags = SonarLEQ
		break
	case token.GEQ:
		flags = SonarGEQ
		break
	default:
		return s // recurse
	}
	// Replace:
	//	x != y
	// with:
	//	func() bool { v1 := x; v2 := y; go-fuzz-dep.Sonar(v1, v2, flags); return v1 != v2 }() == true
	v1 := nn.X
	v2 := nn.Y
	if isUninterestingLiteral(v1) || isUninterestingLiteral(v2) {
		return s
	}
	if isCap(v1) || isCap(v2) {
		// Haven't seen useful cases yet.
		return s
	}
	if isLen(v1) || isLen(v2) {
		// TODO: we could pass both length value and the len argument.
		// For example, if the code is:
		//	name := ... // obtained from input
		//	if len(name) > 5 { ... }
		// If we would have the name value at runtime, we will know
		// what part of the input to alter to affect len result.
		flags |= SonarLength
	}
	if isConstExpr(v1) {
		flags |= SonarConst1
	}
	if isConstExpr(v2) {
		flags |= SonarConst2
	}
	id := int(flags) | sonarSeq<<8
	startPos := s.fset.Position(nn.Pos())
	endPos := s.fset.Position(nn.End())
	*s.blocks = append(*s.blocks, CoverBlock{sonarSeq, s.name, startPos.Line, startPos.Column, endPos.Line, endPos.Column, int(flags)})
	sonarSeq++
	// TODO: walk v1 and v2 recursively
	block := &ast.BlockStmt{}
	if !isSimpleExpr(v1) {
		tmp := ast.NewIdent("v1")
		block.List = append(block.List, &ast.AssignStmt{Tok: token.DEFINE, Lhs: []ast.Expr{tmp}, Rhs: []ast.Expr{v1}})
		v1 = tmp
	}
	if !isSimpleExpr(v2) {
		tmp := ast.NewIdent("v2")
		block.List = append(block.List, &ast.AssignStmt{Tok: token.DEFINE, Lhs: []ast.Expr{tmp}, Rhs: []ast.Expr{v2}})
		v2 = tmp
	}
	block.List = append(block.List,
		&ast.ExprStmt{
			X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: fuzzdepPkg}, Sel: &ast.Ident{Name: "Sonar"}},
				Args: []ast.Expr{v1, v2, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(id)}},
			},
		},
		&ast.ReturnStmt{Results: []ast.Expr{&ast.BinaryExpr{Op: nn.Op, X: v1, Y: v2}}},
	)
	nn.X = &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.Ident{Name: "bool"}}}}},
			Body: block,
		},
	}
	nn.Y = &ast.BasicLit{Kind: token.INT, Value: "true"}
	nn.Op = token.EQL
	return nil
}

func isUninterestingLiteral(n ast.Expr) bool {
	if id, ok := n.(*ast.Ident); ok {
		return id.Name == "nil" || id.Name == "true" || id.Name == "false"
	}
	return false
}

func isSimpleExpr(n ast.Expr) bool {
	switch nn := n.(type) {
	case *ast.Ident:
		return true
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		return isSimpleExpr(nn.X)
	case *ast.BinaryExpr:
		return isSimpleExpr(nn.X) && isSimpleExpr(nn.Y)
	case *ast.SelectorExpr:
		return isSimpleExpr(nn.X) && isSimpleExpr(nn.Sel)
	case *ast.ParenExpr:
		return isSimpleExpr(nn.X)
	case *ast.CallExpr:
		if arg := isConv(nn); arg != nil {
			return isSimpleExpr(arg)
		}
		if isUnsafeOperator(nn) {
			return true
		}
		return false
	default:
		return false
	}
}

func isConstExpr(n ast.Expr) bool {
	switch nn := n.(type) {
	case *ast.Ident:
		return nn.Obj != nil && nn.Obj.Kind == ast.Con
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		return isConstExpr(nn.X)
	case *ast.BinaryExpr:
		return isConstExpr(nn.X) && isConstExpr(nn.Y)
	case *ast.SelectorExpr:
		return isConstExpr(nn.Sel)
	case *ast.ParenExpr:
		return isConstExpr(nn.X)
	case *ast.CallExpr:
		if arg := isConv(nn); arg != nil {
			return isConstExpr(arg)
		}
		if isUnsafeOperator(nn) {
			return true
		}
		return false
	default:
		return false
	}
}

func isCap(n ast.Expr) bool {
	if call, ok := n.(*ast.CallExpr); ok {
		if id, ok2 := call.Fun.(*ast.Ident); ok2 {
			return id.Name == "cap"
		}
	}
	return false
}

func isLen(n ast.Expr) bool {
	if call, ok := n.(*ast.CallExpr); ok {
		if id, ok2 := call.Fun.(*ast.Ident); ok2 {
			return id.Name == "len"
		}
	}
	return false
}

func isConv(n *ast.CallExpr) ast.Expr {
	if id, ok := n.Fun.(*ast.Ident); ok {
		if knownTypes[id.Name] {
			return n.Args[0]
		}
	}
	return nil
}

var knownTypes = map[string]bool{
	"rune":    true,
	"byte":    true,
	"int8":    true,
	"uint8":   true,
	"int16":   true,
	"uint16":  true,
	"int32":   true,
	"uint32":  true,
	"int64":   true,
	"uint64":  true,
	"string":  true,
	"int":     true,
	"uint":    true,
	"intptr":  true,
	"uintptr": true,
	"float32": true,
	"float64": true,
}

func isUnsafeOperator(n *ast.CallExpr) bool {
	if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			return id.Name == "unsafe"
		}
	}
	return false
}

type LiteralCollector struct {
	lits map[Literal]struct{}
}

func (lc *LiteralCollector) Visit(n ast.Node) (w ast.Visitor) {
	switch nn := n.(type) {
	default:
		return lc // recurse
	case *ast.ImportSpec:
		return nil
	case *ast.Field:
		return nil // ignore field tags
	case *ast.CallExpr:
		switch fn := nn.Fun.(type) {
		case *ast.Ident:
			if fn.Name == "panic" {
				return nil
			}
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && (id.Name == "fmt" || id.Name == "errors") {
				return nil
			}
		}
		return lc
	case *ast.BasicLit:
		lit := nn.Value
		switch nn.Kind {
		case token.STRING:
			lc.lits[Literal{unquote(lit), true}] = struct{}{}
		case token.CHAR:
			lc.lits[Literal{unquote(lit), false}] = struct{}{}
		case token.INT:
			if lit[0] < '0' || lit[0] > '9' {
				failf("unsupported literal '%v'", lit)
			}
			v, err := strconv.ParseInt(lit, 0, 64)
			if err != nil {
				u, err := strconv.ParseUint(lit, 0, 64)
				if err != nil {
					failf("failed to parse int literal '%v': %v", lit, err)
				}
				v = int64(u)
			}
			var val []byte
			if v >= -(1<<7) && v < 1<<8 {
				val = append(val, byte(v))
			} else if v >= -(1<<15) && v < 1<<16 {
				val = append(val, byte(v), byte(v>>8))
			} else if v >= -(1<<31) && v < 1<<32 {
				val = append(val, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
			} else {
				val = append(val, byte(v), byte(v>>8), byte(v>>16), byte(v>>24), byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
			}
			lc.lits[Literal{string(val), false}] = struct{}{}
		}
		return nil
	}
}

func trimComments(file *ast.File, fset *token.FileSet) []*ast.CommentGroup {
	var comments []*ast.CommentGroup
	for _, group := range file.Comments {
		var list []*ast.Comment
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:") && fset.Position(comment.Slash).Column == 1 {
				list = append(list, comment)
			}
		}
		if list != nil {
			comments = append(comments, &ast.CommentGroup{list})
		}
	}
	return comments
}

func initialComments(content []byte) []byte {
	// Derived from go/build.Context.shouldBuild.
	end := 0
	p := content
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 { // Blank line.
			end = len(content) - len(p)
			continue
		}
		if !bytes.HasPrefix(line, slashslash) { // Not comment line.
			break
		}
	}
	return content[:end]
}

type File struct {
	fset      *token.FileSet
	name      string // Name of file.
	shortName string
	astFile   *ast.File
	blocks    *[]CoverBlock
}

var slashslash = []byte("//")

func (f *File) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.GenDecl:
		if n.Tok != token.VAR {
			return nil // constants and types are not interesting
		}

	case *ast.BlockStmt:
		// If it's a switch or select, the body is a list of case clauses; don't tag the block itself.
		if len(n.List) > 0 {
			switch n.List[0].(type) {
			case *ast.CaseClause: // switch
				for _, n := range n.List {
					clause := n.(*ast.CaseClause)
					clause.Body = f.addCounters(clause.Pos(), clause.End(), clause.Body, false)
				}
				return f
			case *ast.CommClause: // select
				for _, n := range n.List {
					clause := n.(*ast.CommClause)
					clause.Body = f.addCounters(clause.Pos(), clause.End(), clause.Body, false)
				}
				return f
			}
		}
		n.List = f.addCounters(n.Lbrace, n.Rbrace+1, n.List, true) // +1 to step past closing brace.
	case *ast.IfStmt:
		if n.Init != nil {
			ast.Walk(f, n.Init)
		}
		if n.Cond != nil {
			ast.Walk(f, n.Cond)
		}
		ast.Walk(f, n.Body)
		if n.Else == nil {
			// Add else because we want coverage for "not taken".
			n.Else = &ast.BlockStmt{
				Lbrace: n.Body.End(),
				Rbrace: n.Body.End(),
			}
		}
		// The elses are special, because if we have
		//	if x {
		//	} else if y {
		//	}
		// we want to cover the "if y". To do this, we need a place to drop the counter,
		// so we add a hidden block:
		//	if x {
		//	} else {
		//		if y {
		//		}
		//	}
		switch stmt := n.Else.(type) {
		case *ast.IfStmt:
			block := &ast.BlockStmt{
				Lbrace: n.Body.End(), // Start at end of the "if" block so the covered part looks like it starts at the "else".
				List:   []ast.Stmt{stmt},
				Rbrace: stmt.End(),
			}
			n.Else = block
		case *ast.BlockStmt:
			stmt.Lbrace = n.Body.End() // Start at end of the "if" block so the covered part looks like it starts at the "else".
		default:
			panic("unexpected node type in if")
		}
		ast.Walk(f, n.Else)
		return nil
	case *ast.ForStmt:
		// TODO: handle increment statement
	case *ast.SelectStmt:
		// Don't annotate an empty select - creates a syntax error.
		if n.Body == nil || len(n.Body.List) == 0 {
			return nil
		}
	case *ast.SwitchStmt:
		hasDefault := false
		if n.Body == nil {
			n.Body = new(ast.BlockStmt)
		}
		for _, s := range n.Body.List {
			if cas, ok := s.(*ast.CaseClause); ok && cas.List == nil {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			// Add default case to get additional coverage.
			n.Body.List = append(n.Body.List, &ast.CaseClause{})
		}

		// Don't annotate an empty switch - creates a syntax error.
		if n.Body == nil || len(n.Body.List) == 0 {
			return nil
		}
	case *ast.TypeSwitchStmt:
		// Don't annotate an empty type switch - creates a syntax error.
		// TODO: add default case
		if n.Body == nil || len(n.Body.List) == 0 {
			return nil
		}
	case *ast.BinaryExpr:
		if n.Op == token.LAND || n.Op == token.LOR {
			// Replace:
			//	x && y
			// with:
			//	x && func() bool { return y }
			n.Y = &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.Ident{Name: "bool"}}}}},
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{n.Y}}}},
				},
			}
		}
	}
	return f
}

func (f *File) addImport(path, name, anyIdent string) {
	newImport := &ast.ImportSpec{
		Name: ast.NewIdent(name),
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf("%q", path),
		},
	}
	impDecl := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			newImport,
		},
	}
	// Make the new import the first Decl in the file.
	astFile := f.astFile
	astFile.Decls = append(astFile.Decls, nil)
	copy(astFile.Decls[1:], astFile.Decls[0:])
	astFile.Decls[0] = impDecl
	astFile.Imports = append(astFile.Imports, newImport)

	// Now refer to the package, just in case it ends up unused.
	// That is, append to the end of the file the declaration
	//	var _ = _cover_atomic_.AddUint32
	reference := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{
			&ast.ValueSpec{
				Names: []*ast.Ident{
					ast.NewIdent("_"),
				},
				Values: []ast.Expr{
					&ast.SelectorExpr{
						X:   ast.NewIdent(name),
						Sel: ast.NewIdent(anyIdent),
					},
				},
			},
		},
	}
	astFile.Decls = append(astFile.Decls, reference)
}

func (f *File) addCounters(pos, blockEnd token.Pos, list []ast.Stmt, extendToClosingBrace bool) []ast.Stmt {
	// Special case: make sure we add a counter to an empty block. Can't do this below
	// or we will add a counter to an empty statement list after, say, a return statement.
	if len(list) == 0 {
		return []ast.Stmt{f.newCounter(pos, blockEnd, 0)}
	}
	// We have a block (statement list), but it may have several basic blocks due to the
	// appearance of statements that affect the flow of control.
	var newList []ast.Stmt
	for {
		// Find first statement that affects flow of control (break, continue, if, etc.).
		// It will be the last statement of this basic block.
		var last int
		end := blockEnd
		for last = 0; last < len(list); last++ {
			end = f.statementBoundary(list[last])
			if f.endsBasicSourceBlock(list[last]) {
				extendToClosingBrace = false // Block is broken up now.
				last++
				break
			}
		}
		if extendToClosingBrace {
			end = blockEnd
		}
		if pos != end { // Can have no source to cover if e.g. blocks abut.
			newList = append(newList, f.newCounter(pos, end, last))
		}
		newList = append(newList, list[0:last]...)
		list = list[last:]
		if len(list) == 0 {
			break
		}
		pos = list[0].Pos()
	}
	return newList
}

func (f *File) endsBasicSourceBlock(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.BlockStmt:
		// Treat blocks like basic blocks to avoid overlapping counters.
		return true
	case *ast.BranchStmt:
		return true
	case *ast.ForStmt:
		return true
	case *ast.IfStmt:
		return true
	case *ast.LabeledStmt:
		return f.endsBasicSourceBlock(s.Stmt)
	case *ast.RangeStmt:
		return true
	case *ast.SwitchStmt:
		return true
	case *ast.SelectStmt:
		return true
	case *ast.TypeSwitchStmt:
		return true
	case *ast.ExprStmt:
		// Calls to panic change the flow.
		// We really should verify that "panic" is the predefined function,
		// but without type checking we can't and the likelihood of it being
		// an actual problem is vanishingly small.
		if call, ok := s.X.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "panic" && len(call.Args) == 1 {
				return true
			}
		}
	}
	found, _ := hasFuncLiteral(s)
	return found
}

func (f *File) statementBoundary(s ast.Stmt) token.Pos {
	// Control flow statements are easy.
	switch s := s.(type) {
	case *ast.BlockStmt:
		// Treat blocks like basic blocks to avoid overlapping counters.
		return s.Lbrace
	case *ast.IfStmt:
		found, pos := hasFuncLiteral(s.Init)
		if found {
			return pos
		}
		found, pos = hasFuncLiteral(s.Cond)
		if found {
			return pos
		}
		return s.Body.Lbrace
	case *ast.ForStmt:
		found, pos := hasFuncLiteral(s.Init)
		if found {
			return pos
		}
		found, pos = hasFuncLiteral(s.Cond)
		if found {
			return pos
		}
		found, pos = hasFuncLiteral(s.Post)
		if found {
			return pos
		}
		return s.Body.Lbrace
	case *ast.LabeledStmt:
		return f.statementBoundary(s.Stmt)
	case *ast.RangeStmt:
		found, pos := hasFuncLiteral(s.X)
		if found {
			return pos
		}
		return s.Body.Lbrace
	case *ast.SwitchStmt:
		found, pos := hasFuncLiteral(s.Init)
		if found {
			return pos
		}
		found, pos = hasFuncLiteral(s.Tag)
		if found {
			return pos
		}
		return s.Body.Lbrace
	case *ast.SelectStmt:
		return s.Body.Lbrace
	case *ast.TypeSwitchStmt:
		found, pos := hasFuncLiteral(s.Init)
		if found {
			return pos
		}
		return s.Body.Lbrace
	}
	found, pos := hasFuncLiteral(s)
	if found {
		return pos
	}
	return s.End()
}

var counterGen uint32

func genCounter() int {
	counterGen++
	id := counterGen
	buf := []byte{byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)}
	hash := sha1.Sum(buf)
	return int(uint16(hash[0]) | uint16(hash[1])<<8)
}

func (f *File) newCounter(start, end token.Pos, numStmt int) ast.Stmt {
	cnt := genCounter()

	if f.blocks != nil {
		s := f.fset.Position(start)
		e := f.fset.Position(end)
		*f.blocks = append(*f.blocks, CoverBlock{cnt, f.shortName, s.Line, s.Column, e.Line, e.Column, numStmt})
	}

	idx := &ast.BasicLit{
		Kind:  token.INT,
		Value: strconv.Itoa(cnt),
	}
	counter := &ast.IndexExpr{
		X: &ast.SelectorExpr{
			X:   ast.NewIdent(fuzzdepPkg),
			Sel: ast.NewIdent("CoverTab"),
		},
		Index: idx,
	}
	return &ast.IncDecStmt{
		X:   counter,
		Tok: token.INC,
	}
}

func (f *File) print(w io.Writer) {
	printer.Fprint(w, f.fset, f.astFile)
}

type funcLitFinder token.Pos

func (f *funcLitFinder) Visit(node ast.Node) (w ast.Visitor) {
	if f.found() {
		return nil // Prune search.
	}
	switch n := node.(type) {
	case *ast.FuncLit:
		*f = funcLitFinder(n.Body.Lbrace)
		return nil // Prune search.
	}
	return f
}

func (f *funcLitFinder) found() bool {
	return token.Pos(*f) != token.NoPos
}

func hasFuncLiteral(n ast.Node) (bool, token.Pos) {
	if n == nil {
		return false, 0
	}
	var literal funcLitFinder
	ast.Walk(&literal, n)
	return literal.found(), token.Pos(literal)
}

func unquote(s string) string {
	t, err := strconv.Unquote(s)
	if err != nil {
		failf("cover: improperly quoted string %q\n", s)
	}
	return t
}
