// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package sqlparser

import (
	"fmt"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
	"github.com/youtube/vitess/go/vt/sqlparser"
)

func Fuzz(data []byte) int {
	stmt, err := sqlparser.Parse(string(data))
	if err != nil {
		if stmt != nil {
			panic("stmt is not nil on error")
		}
		return 0
	}
	if true {
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
	} else {
		sqlparser.String(stmt)
	}
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
				sqlparser.GetColName(x)
			}
			if x, ok := n.(sqlparser.ValExpr); ok {
				sqlparser.IsValue(x)
			}
			if x, ok := n.(sqlparser.ValExpr); ok {
				sqlparser.IsColName(x)
			}
			if x, ok := n.(sqlparser.ValExpr); ok {
				sqlparser.IsSimpleTuple(x)
			}
			if x, ok := n.(sqlparser.ValExpr); ok {
				sqlparser.AsInterface(x)
			}
			if x, ok := n.(sqlparser.BoolExpr); ok {
				sqlparser.HasINClause([]sqlparser.BoolExpr{x})
			}
		}
	}
	buf := sqlparser.NewTrackedBuffer(nil)
	stmt.Format(buf)
	pq := buf.ParsedQuery()
	vars := map[string]interface{}{
		"A": 42,
		"B": 123123123,
		"C": "",
		"D": "a",
		"E": "foobar",
		"F": 1.1,
	}
	pq.GenerateQuery(vars)
	return 1
}
