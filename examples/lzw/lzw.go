package lzw

import (
	"bytes"
	"compress/lzw"
	"io/ioutil"
)

const (
	order = lzw.MSB
	width = 3
)

func Fuzz(data []byte) int {
	r := lzw.NewReader(bytes.NewReader(data), order, width)
	data1, err := ioutil.ReadAll(r)
	if err != nil {
		return 0
	}
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, order, width)
	n, err := w.Write(data1)
	if err != nil {
		panic("write failed")
	}
	if n != len(data1) {
		panic("invalid write length")
	}
	w.Close()
	r1 := lzw.NewReader(&buf, order, width)
	data2, err := ioutil.ReadAll(r1)
	if err != nil {
		panic("failed to decompress")
	}
	if !bytes.Equal(data1, data2) {
		panic("corrputed data")
	}
	return 1
}
