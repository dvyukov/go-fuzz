// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package gorillamux

import (
	"bufio"
	"github.com/gorilla/mux"
	"net/http"
	"strings"
)

func Fuzz(data []byte) int {
	s := string(data)
	r0 := s[:len(s)/3]
	r1 := s[len(s)/3 : len(s)/3*2]
	reqs := s[len(s)/3*2:]
	r := mux.NewRouter()
	r.HandleFunc(r0, foo)
	r.HandleFunc(r1, foo)
	if req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(reqs))); err == nil {
		var match mux.RouteMatch
		r.Match(req, &match)
	}
	return 0
}

func foo(w http.ResponseWriter, r *http.Request) {
}
