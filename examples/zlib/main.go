package zlib

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	_, err = ioutil.ReadAll(r)
	if err != nil {
		return 0
	}
	return 1
}
