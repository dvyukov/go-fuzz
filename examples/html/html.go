// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package html

import (
	"bytes"
	"golang.org/x/net/html"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	nodes, err := html.ParseFragment(bytes.NewReader(data), nil)
	if err != nil {
		return 0
	}
	for _, n := range nodes {
		if err := html.Render(ioutil.Discard, n); err != nil {
			panic(err)
		}
	}
	return 1
}
