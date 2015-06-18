package gofmt

import (
	"bytes"
	"fmt"
	"go/format"
	"unicode/utf8"
)

func Fuzz(data []byte) int {
	if bytes.Contains(data, []byte("//line")) {
		// https://github.com/golang/go/issues/11276
		return 0
	}
	if bytes.IndexByte(data, ';') != -1 {
		// https://github.com/golang/go/issues/11274
		return 0
	}
	if bytes.IndexByte(data, '\r') != -1 {
		// https://github.com/golang/go/issues/11151
		return 0
	}
	if !utf8.Valid(data) {
		return 0
	}
	data1, err := format.Source(data)
	if err != nil {
		if data1 != nil {
			panic("data is not nil on error")
		}
		return 0
	}
	data2, err := format.Source(data1)
	if err != nil {
		fmt.Printf("orig: %q\n", data)
		fmt.Printf("new : %q\n", data1)
		panic(err)
	}
	if !bytes.Equal(data1, data2) {
		fmt.Printf("orig: %q\n", data)
		fmt.Printf("new1: %q\n", data1)
		fmt.Printf("new2: %q\n", data2)
		panic("non-idempotent format")
	}
	can := canonical(data)
	can1 := canonical(data1)
	if !bytes.Equal(can, can1) {
		fmt.Printf("orig: %q\n", data)
		fmt.Printf("new : %q\n", data1)
		panic("corrupting format")
	}
	return 1
}

func canonical(data []byte) []byte {
	for i := 0; i <= ' '; i++ {
		data = bytes.Replace(data, []byte{byte(i)}, nil, -1)
	}
	data = bytes.Replace(data, []byte{';'}, nil, -1)
	return data
}
