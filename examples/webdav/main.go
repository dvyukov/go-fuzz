// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package webdav

import (
	"bufio"
	"bytes"
	"net/http"

	"golang.org/x/net/webdav"
)

func Fuzz(data []byte) int {
	score := 0
	buf := bufio.NewReader(bytes.NewReader(data))
	for {
		r, err := http.ReadRequest(buf)
		if err != nil {
			break
		}
		h := &webdav.Handler{
			FileSystem: webdav.NewMemFS(),
			LockSystem: webdav.NewMemLS(),
		}
		w := &NilWriter{hdr: make(http.Header)}
		h.ServeHTTP(w, r)
		score = 1
	}
	return score
}

type NilWriter struct {
	hdr  http.Header
	code int
}

func (w *NilWriter) Header() http.Header {
	return w.hdr
}

func (w *NilWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *NilWriter) WriteHeader(code int) {
	w.code = code
}
