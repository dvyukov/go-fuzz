// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package zip

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	// Read in the archive.
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if z != nil {
			panic("non nil z")
		}
		return 0
	}
	var headers []*zip.FileHeader
	var contents [][]byte
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			continue
		}
		if f.UncompressedSize64 < 1e6 {
			c, err := ioutil.ReadAll(r)
			if err != nil {
				continue
			}
			if uint64(len(c)) != f.UncompressedSize64 {
				println("bad size:", len(c), f.UncompressedSize64)
				panic("bad size")
			}
			hdr := f.FileHeader
			headers = append(headers, &hdr)
			contents = append(contents, c)
		}
		r.Close()
	}
	if len(headers) == 0 {
		return 1
	}

	// Write a new archive with the same files.
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for i, h := range headers {
		w1, err := w.CreateHeader(h)
		if err != nil {
			panic(err)
		}
		n, err := w1.Write(contents[i])
		if err != nil {
			panic(err)
		}
		if n != len(contents[i]) {
			panic("short write")
		}
	}
	err = w.Close()
	if err != nil {
		panic(err)
	}

	// Read in the new archive.
	z1, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(len(buf.Bytes())))
	if err != nil {
		panic(err)
	}
	var headers1 []*zip.FileHeader
	var contents1 [][]byte
	for _, f := range z1.File {
		r, err := f.Open()
		if err != nil {
			panic(err)
		}
		if f.UncompressedSize64 >= 1e6 {
			panic("corrupted length")
		}
		c, err := ioutil.ReadAll(r)
		if err != nil {
			panic(err)
		}
		if uint64(len(c)) != f.UncompressedSize64 {
			println("bad size:", len(c), f.UncompressedSize64)
			panic("bad size")
		}
		hdr := f.FileHeader
		headers1 = append(headers1, &hdr)
		contents1 = append(contents1, c)
		r.Close()
	}

	// Compare that we have the same data after compress/decompress.
	for i, h := range headers {
		// These fields are set by archive/zip package.
		h.Flags |= 0x8
		h.CreatorVersion = headers1[i].CreatorVersion
		h.ReaderVersion = headers1[i].ReaderVersion
		// These are not set correctly initially.
		//h.CompressedSize = headers1[i].CompressedSize
		//h.CompressedSize64 = headers1[i].CompressedSize64
		if !fuzz.DeepEqual(h, headers1[i]) {
			fmt.Printf("hdr0: %#v\n", h)
			fmt.Printf("hdr1: %#v\n", headers1[i])
			panic("corrupted header")
		}
		if !fuzz.DeepEqual(contents[i], contents1[i]) {
			panic("corrupted data")
		}
	}
	return 1
}
