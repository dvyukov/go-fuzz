// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package snappy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/golang/snappy"
)

func Fuzz(data []byte) int {
	n, err := snappy.DecodedLen(data)
	if err != nil || n > 1e6 {
		return 0
	}
	if n < 0 {
		panic("negative decoded len")
	}
	dec, err := snappy.Decode(nil, data)
	if err != nil {
		if dec != nil {
			panic("dec is not nil")
		}
		return 0
	}
	if len(dec) != n {
		println(len(dec), n)
		panic("bad decoded len")
	}
	n = snappy.MaxEncodedLen(len(dec))
	enc := snappy.Encode(nil, dec)
	if len(enc) > n {
		panic("bad encoded len")
	}

	dec1, err := snappy.Decode(nil, enc)
	if err != nil {
		panic(err)
	}
	if bytes.Compare(dec, dec1) != 0 {
		panic("not equal")
	}
	return 1
}

func FuzzFraming(data []byte) int {
	r := snappy.NewReader(bytes.NewReader(data))
	buf := make([]byte, 0, 1023)
	dec := make([]byte, 0, 1024)
	for i := 0; ; i++ {
		x := i
		if x > cap(buf) {
			x = cap(buf)
		}
		n, err := r.Read(buf[:x])
		if n != 0 {
			dec = append(dec, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0
		}
	}
	r.Reset(bytes.NewReader(data))
	dec1, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	if bytes.Compare(dec, dec1) != 0 {
		fmt.Printf("dec0: %q\n", dec)
		fmt.Printf("dec1: %q\n", dec1)
		panic("not equal")
	}

	bufw := new(bytes.Buffer)
	w := snappy.NewBufferedWriter(bufw)
	for i := 0; len(dec1) > 0; i++ {
		x := i
		if x > len(dec1) {
			x = len(dec1)
		}
		n, err := w.Write(dec1[:x])
		if n != x {
			panic("short write")
		}
		if err != nil {
			panic(err)
		}
		dec1 = dec1[x:]
		if (i % 2) != 0 {
			w.Flush()
		}
	}
	w.Close()

	dec1 = append([]byte{}, dec...)
	bufw2 := new(bytes.Buffer)
	w2 := snappy.NewWriter(bufw2)
	for i := 2; len(dec1) > 0; i++ {
		x := i
		if x > len(dec1) {
			x = len(dec1)
		}
		n, err := w2.Write(dec1[:x])
		if n != x {
			panic("short write")
		}
		if err != nil {
			panic(err)
		}
		dec1 = dec1[x:]
		if (i % 2) != 0 {
			w2.Flush()
		}
	}
	w2.Close()

	r2 := snappy.NewReader(bufw)
	dec2, err := ioutil.ReadAll(r2)
	if err != nil {
		panic(err)
	}
	if bytes.Compare(dec, dec2) != 0 {
		panic("not equal")
	}

	r3 := snappy.NewReader(bufw2)
	dec3, err := ioutil.ReadAll(r3)
	if err != nil {
		panic(err)
	}
	if bytes.Compare(dec, dec3) != 0 {
		panic("not equal")
	}

	return 1
}
