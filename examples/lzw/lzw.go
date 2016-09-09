// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package lzw

import (
	"bytes"
	"compress/lzw"
	"fmt"
	"io"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	r := lzw.NewReader(bytes.NewReader(data), lzw.MSB, 8)
	uncomp := make([]byte, 64<<10)
	n, err := r.Read(uncomp)
	if err != nil && err != io.EOF {
		return 0
	}
	if n == len(uncomp) {
		return 0 // too large
	}
	uncomp = uncomp[:n]
	for width := 2; width <= 8; width++ {
		uncomp0 := append([]byte{}, uncomp...)
		for i, v := range uncomp0 {
			uncomp0[i] = v & (1<<uint(width) - 1)
		}
		for _, order := range []lzw.Order{lzw.MSB, lzw.LSB} {
			buf := new(bytes.Buffer)
			w := lzw.NewWriter(buf, order, width)
			n, err := w.Write(uncomp0)
			if err != nil {
				fmt.Printf("order=%v width=%v\n", order, width)
				panic(err)
			}
			if n != len(uncomp0) {
				fmt.Printf("order=%v width=%v\n", order, width)
				panic("short write")
			}
			if err := w.Close(); err != nil {
				fmt.Printf("order=%v width=%v\n", order, width)
				panic(err)
			}
			r1 := lzw.NewReader(buf, order, width)
			uncomp1, err := ioutil.ReadAll(r1)
			if err != nil {
				fmt.Printf("order=%v width=%v\n", order, width)
				panic(err)
			}
			if !bytes.Equal(uncomp0, uncomp1) {
				fmt.Printf("order=%v width=%v\n", order, width)
				panic("data differs")
			}
		}
	}
	return 1
}
