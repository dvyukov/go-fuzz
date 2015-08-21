// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package httpreq

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	r, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return 0
	}
	r.ParseMultipartForm(1e6)
	r.ParseForm()
	r.BasicAuth()
	r.Cookies()
	r.FormFile("foo")
	r.FormValue("bar")
	r.PostFormValue("baz")
	r.MultipartReader()
	r.Referer()
	r.UserAgent()

	// Read it in again, because we consumed body.
	r, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		panic(err)
	}
	if _, err := ioutil.ReadAll(r.Body); err != nil {
		return 0
	}

	// Read it in again, because we consumed body.
	r, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		panic(err)
	}
	if err := r.Write(ioutil.Discard); err != nil {
		panic(err)
	}

	// Read it in again, because we consumed body.
	r0, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		panic(err)
	}
	// Write will set these.
	fix := func(r0 *http.Request) {
		if r0.Header.Get("User-Agent") == "" {
			r0.Header.Set("User-Agent", "Go 1.1 package http")
		}
		if len(r0.Header["User-Agent"]) > 1 {
			r0.Header["User-Agent"] = r0.Header["User-Agent"][:1]
		}
		if r0.Method == "" {
			r0.Method = "GET"
		}
		if r0.Method == "CONNECT" {
			// This won't be send on CONNECT.
			r0.URL.User = nil
			r0.URL.Host = ""
		}
		if r0.URL.Scheme != "" && r0.URL.Opaque == "" {
			// User is not sent in such for some reason.
			r0.URL.User = nil
			r0.URL.Path = ""
			r0.URL.Fragment = ""
		}
		if h := r0.URL.Host; len(h) > 0 && h[len(h)-1] == '%' {
			r0.URL.Host = "aaa"
		}
		// https://github.com/golang/go/issues/11208
		r0.Host = strings.Replace(r0.Host, "%", "a", -1)
		r0.URL.Opaque = strings.Replace(r0.URL.Opaque, "%", "a", -1)
		r0.Trailer = nil
		r0.ProtoMajor = 1
		r0.ProtoMinor = 1
		r0.Header.Del("Trailer")
		r0.Header.Del("Connection")
		for k, v := range r0.Header {
			for i := range v {
				// https://github.com/golang/go/issues/11204
				v[i] = strings.Trim(v[i], " \t\r\n")
				// https://github.com/golang/go/issues/11207
				v[i] = strings.Replace(v[i], "\r", " ", -1)
			}
			r0.Header[k] = v
		}
	}
	fix(r0)

	buf := new(bytes.Buffer)
	if err := r0.WriteProxy(buf); err != nil {
		panic(err)
	}
	data1 := buf.Bytes()
	r1, err := http.ReadRequest(bufio.NewReader(buf))
	if err != nil {
		// https://github.com/golang/go/issues/11202
		// https://github.com/golang/go/issues/11203
		if strings.Contains(err.Error(), "invalid URI for request") {
			return 0
		}
		if strings.Contains(err.Error(), "missing protocol scheme") {
			return 0
		}
		// https://github.com/golang/go/issues/11206
		if strings.Contains(err.Error(), "malformed HTTP version") {
			return 0
		}
		fmt.Printf("req0: %q\nURL: %#v\n", data, *r0.URL)
		fmt.Printf("req1: %q\n", data1)
		panic(err)
	}
	// Read it in again, because we consumed body.
	r0, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		panic(err)
	}
	fix(r0)
	r0.RequestURI = r1.RequestURI
	r0.Proto = r1.Proto
	if r1.Header.Get("Connection") != "" {
		r0.Header.Set("Connection", r1.Header.Get("Connection"))
	} else {
		r0.Header.Del("Connection")
		r1.Header.Del("Connection")
	}
	if r1.Header.Get("Content-Length") != "" {
		r0.Header.Set("Content-Length", r1.Header.Get("Content-Length"))
	} else {
		r0.Header.Del("Content-Length")
		r1.Header.Del("Content-Length")
	}
	if r0.URL.Path == "" && r1.URL.Path == "/" {
		r0.URL.Path = r1.URL.Path
	}
	// Rules for host are too complex.
	r0.Host = ""
	r1.Host = ""
	r0.URL.Host = ""
	r1.URL.Host = ""
	if (r0.URL.Scheme != "" && r0.URL.Opaque != "") || r0.Method == "CONNECT" {
		r0.URL = nil
		r1.URL = nil
	}
	body0, err := ioutil.ReadAll(r0.Body)
	if err != nil {
		panic(err)
	}
	body1, err := ioutil.ReadAll(r1.Body)
	if err != nil {
		panic(err)
	}
	if !bytes.Equal(body0, body1) {
		fmt.Printf("body0: %q\n", body0)
		fmt.Printf("body1: %q\n", body1)
		panic("body changed")
	}
	r0.Body = nil
	r1.Body = nil
	if !fuzz.DeepEqual(r0, r1) {
		fmt.Printf("req0: %#v\n", *r0)
		fmt.Printf("req1: %#v\n", *r1)
		fmt.Printf("url0: %#v\n", *r0.URL)
		fmt.Printf("url1: %#v\n", *r1.URL)
		panic("not equal")
	}
	return 1
}
