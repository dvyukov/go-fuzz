package gzip

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	r := bytes.NewReader(data)
	fr, err := gzip.NewReader(r)
	if err != nil {
		return 0
	}
	ioutil.ReadAll(fr)
	return 1
}
