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
	var b bytes.Buffer
	fw, _ := flate.NewWriter(&b, 8)
	fw.Write(data1)
	fw.Close()

	fr1 := flate.NewReader(&b)
	data2, err := ioutil.ReadAll(fr1)
	if err != nil {
		panic(err)
	}
	if bytes.Compare(data1, data2) != 0 {
		panic("not equal")
	}
	return 1
}
