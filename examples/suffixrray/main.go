// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package suffix

import (
	"bytes"
	"fmt"
	"sort"
	"index/suffixarray"
)

func Fuzz(data []byte) int {
	test(data, what1)
	test(data, what2)
	test(data, what3)
	return 0
}

func test(data, what []byte) {
	s := suffixarray.New(data)
	idx0 := s.Lookup(what, -1)
	idx1 := simple(data, what)
	if len(idx0) != len(idx1) {
		panic(fmt.Sprintf("len mismatch: %+v, %+v", idx0, idx1))
	}
	sort.Ints(idx0)
	for i, x := range idx0 {
		if x != idx1[i] {
			panic(fmt.Sprintf("data mismatch: %+v, %+v", idx0, idx1))
		}
	}
}

func simple(data, what []byte) []int {
	var res []int
	pos := 0
	for {
		x := bytes.Index(data[pos:], what)
		if x == -1 {
			break
		}
		res = append(res, pos+x)
		pos += x + 1
	}
	return res
}

var what1 = []byte("f")
var what2 = []byte("oo")
var what3 = []byte("foo")
