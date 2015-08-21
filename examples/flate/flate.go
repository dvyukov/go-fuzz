// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package flate

import (
	"bytes"
	"compress/flate"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	r := bytes.NewReader(data)
	fr := flate.NewReader(r)
	data1, err := ioutil.ReadAll(fr)
	if _, ok := err.(flate.InternalError); ok {
		panic(err)
	}
	if err != nil {
		return 0
	}
	for level := 0; level <= 9; level++ {
		buf := new(bytes.Buffer)
		fw, err := flate.NewWriter(buf, level)
		if err != nil {
			panic(err)
		}
		n, err := fw.Write(data1)
		if n != len(data1) {
			panic("short write")
		}
		if err != nil {
			panic(err)
		}
		err = fw.Close()
		if err != nil {
			panic(err)
		}
		fr1 := flate.NewReader(buf)
		data2, err := ioutil.ReadAll(fr1)
		if err != nil {
			panic(err)
		}
		if bytes.Compare(data1, data2) != 0 {
			panic("not equal")
		}
	}
	return 1
}
