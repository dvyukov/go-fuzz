// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package sqlparser

import (
	"bytes"
	"fmt"
	"io"

	"github.com/youtube/vitess/go/sqltypes"
	querypb "github.com/youtube/vitess/go/vt/proto/query"
	"github.com/youtube/vitess/go/vt/sqlparser"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

// shortReader is a io.Reader that forces all reads to only be
// a few bytes at a time. This helps force more code paths in
// the parser which makes heavy use of an internal buffer.
type shortReader struct {
	r io.Reader
	n int
}

func (r *shortReader) Read(p []byte) (n int, err error) {
	if len(p) > r.n {
		p = p[:r.n]
	}
	return r.r.Read(p)
}

func parseAll(data []byte) ([]sqlparser.Statement, error) {
	r := &shortReader{
		r: bytes.NewReader(data),
		n: 3,
	}

	tokens := sqlparser.NewTokenizer(r)

	var statements []sqlparser.Statement
	for i := 0; i < 1000; i++ { // Only allow 1000 statements
		if stmt, err := sqlparser.ParseNext(tokens); err != nil {
			if stmt != nil {
				panic("stmt is not nil on error")
			}
			if err == io.EOF {
				err = nil
			}
			return statements, err
		} else {
			statements = append(statements, stmt)
		}
	}

	panic("ParseNext loop")
}

// stringAndParse turns the Statement into a SQL string, re-parses
// that string, and checks the result matches the original.
func stringAndParse(data []byte, stmt sqlparser.Statement) {
	data1 := sqlparser.String(stmt)
	stmt1, err := sqlparser.Parse(data1)
	if err != nil {
		fmt.Printf("data0: %q\n", data)
		fmt.Printf("data1: %q\n", data1)
		panic(err)
	}
	if !fuzz.DeepEqual(stmt, stmt1) {
		fmt.Printf("data0: %q\n", data)
		fmt.Printf("data1: %q\n", data1)
		panic("not equal")
	}
}

func Fuzz(data []byte) int {
	stmts, err := parseAll(data)
	if err != nil {
		return 0
	}
	for _, stmt := range stmts {
		stringAndParse(data, stmt)

		if sel, ok := stmt.(*sqlparser.Select); ok {
			var nodes []sqlparser.SQLNode
			for _, x := range sel.From {
				nodes = append(nodes, x)
			}
			for _, x := range sel.SelectExprs {
				nodes = append(nodes, x)
			}
			for _, x := range sel.GroupBy {
				nodes = append(nodes, x)
			}
			for _, x := range sel.OrderBy {
				nodes = append(nodes, x)
			}
			nodes = append(nodes, sel.Where)
			nodes = append(nodes, sel.Having)
			nodes = append(nodes, sel.Limit)
			for _, n := range nodes {
				if n == nil {
					continue
				}
				if x, ok := n.(sqlparser.SimpleTableExpr); ok {
					sqlparser.GetTableName(x)
				}
				if x, ok := n.(sqlparser.Expr); ok {
					sqlparser.IsColName(x)
					sqlparser.IsValue(x)
					sqlparser.IsNull(x)
					sqlparser.IsSimpleTuple(x)
				}
			}
		}
		pq := sqlparser.NewParsedQuery(stmt)
		vars := map[string]*querypb.BindVariable{
			"A": sqltypes.Int64BindVariable(42),
			"B": sqltypes.Uint64BindVariable(123123123),
			"C": sqltypes.StringBindVariable("aa"),
			"D": sqltypes.BytesBindVariable([]byte("a")),
			"E": sqltypes.StringBindVariable("foobar"),
			"F": sqltypes.Float64BindVariable(1.1),
		}
		pq.GenerateQuery(vars, nil)
	}
	return 1
}
