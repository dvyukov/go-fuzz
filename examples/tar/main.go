// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package tar

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	t := tar.NewReader(bytes.NewReader(data))
	var headers []*tar.Header
	var contents [][]byte
	for {
		hdr, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0
		}
		if len(hdr.Name) > 1e6 ||
			len(hdr.Linkname) > 1e6 ||
			len(hdr.Uname) > 1e6 ||
			len(hdr.Gname) > 1e6 ||
			len(hdr.Xattrs) > 1e6 {
			panic("huge header data")
		}
		if hdr.Size > 1e6 {
			panic("huge claimed file size")
		}
		fdata, err := ioutil.ReadAll(t)
		if err != nil {
			return 0
		}
		if int64(len(fdata)) > hdr.Size {
			panic("long read")
		}
		if int64(len(fdata)) < hdr.Size {
			panic("short read")
		}
		hdr1 := *hdr // make a copy to be safe
		headers = append(headers, &hdr1)
		contents = append(contents, fdata)
	}
	buf := new(bytes.Buffer)
	w := tar.NewWriter(buf)
	for i, hdr := range headers {
		err := w.WriteHeader(hdr)
		if err != nil {
			panic(err)
		}
		n, err := w.Write(contents[i])
		if err != nil {
			panic(err)
		}
		if n != len(contents[i]) {
			panic("short write")
		}
	}
	err := w.Close()
	if err != nil {
		panic(err)
	}
	t1 := tar.NewReader(buf)
	for i := 0; ; i++ {
		hdr, err := t1.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fdata, err := ioutil.ReadAll(t)
		if err != nil {
			panic(err)
		}
		if !fuzz.DeepEqual(hdr, headers[i]) {
			panic("headers diffs")
		}
		if !bytes.Equal(fdata, contents[i]) {
			panic("data diffs")
		}
	}
	return 1
}
