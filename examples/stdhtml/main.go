// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package stdhtml

import (
	"fmt"
	"html"
)

func Fuzz(data []byte) int {
	s0 := string(data)
	s1 := html.EscapeString(s0)
	s2 := html.UnescapeString(s1)
	if s0 != s2 {
		fmt.Printf("s0: %q\n", s0)
		fmt.Printf("s1: %q\n", s1)
		fmt.Printf("s2: %q\n", s2)
		panic("string changed")
	}
	s3 := html.UnescapeString(s0)
	html.EscapeString(s3)
	return 0
}
